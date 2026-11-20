// Package geo turns an IP address into a country, region and city with an
// offline database (DB-IP Lite, free, CC BY 4.0, downloaded once a month, or
// any MaxMind-format file you give it). Nothing leaves the box per visit.
package geo

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

// Place is where an address is.
type Place struct {
	Country string // ISO 3166-1 alpha-2
	Region  string
	City    string
}

type record struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Subdivisions []struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
}

// DB looks addresses up; it is safe to use while a newer file loads.
type DB struct {
	kind  string // city | country | off
	path  string // a file given by the admin (never downloaded over)
	dir   string
	ua    string
	log   *slog.Logger
	r     atomic.Pointer[maxminddb.Reader]
	state atomic.Value // string: what Settings → About shows
}

// New prepares the lookups: kind is city | country | off; path is an own
// .mmdb file ("" = download DB-IP Lite into dir).
func New(kind, path, dir, userAgent string, log *slog.Logger) *DB {
	d := &DB{kind: kind, path: path, dir: dir, ua: userAgent, log: log}
	d.state.Store("off")
	return d
}

// Start opens what is there and keeps it fresh (a check a day).
func (d *DB) Start(ctx context.Context) {
	if d.kind == "off" || d.kind == "" {
		return
	}
	if d.path != "" {
		d.open(d.path)
		return
	}
	file := d.file()
	if _, err := os.Stat(file); err == nil {
		d.open(file)
	} else {
		d.state.Store("downloading")
	}
	go func() {
		for {
			d.refresh(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(24 * time.Hour):
			}
		}
	}()
}

func (d *DB) file() string { return filepath.Join(d.dir, "dbip-"+d.kind+"-lite.mmdb") }

func (d *DB) open(path string) {
	r, err := maxminddb.Open(path)
	if err != nil {
		d.log.Warn("geo: cannot open the database", "path", path, "err", err)
		d.state.Store("error: " + err.Error())
		return
	}
	if old := d.r.Swap(r); old != nil {
		go func() { time.Sleep(time.Minute); old.Close() }()
	}
	d.state.Store(fmt.Sprintf("ready (%s, %s)", filepath.Base(path), time.Unix(int64(r.Metadata.BuildEpoch), 0).UTC().Format("2006-01-02")))
}

// refresh downloads this month's file when the one there is older than 32 days.
func (d *DB) refresh(ctx context.Context) {
	file := d.file()
	if fi, err := os.Stat(file); err == nil && time.Since(fi.ModTime()) < 32*24*time.Hour {
		return
	}
	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return
	}
	now := time.Now().UTC()
	var err error
	for _, m := range []time.Time{now, now.AddDate(0, -1, 0)} {
		url := fmt.Sprintf("https://download.db-ip.com/free/dbip-%s-lite-%s.mmdb.gz", d.kind, m.Format("2006-01"))
		if err = d.download(ctx, url, file); err == nil {
			d.log.Info("geo: database downloaded", "url", url)
			d.open(file)
			return
		}
	}
	d.log.Warn("geo: download failed (places stay empty until it works)", "err", err)
	if d.r.Load() == nil {
		d.state.Store("download failed: " + err.Error())
	}
}

func (d *DB) download(ctx context.Context, url, file string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", d.ua)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, res.Status)
	}
	zr, err := gzip.NewReader(res.Body)
	if err != nil {
		return err
	}
	tmp := file + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, io.LimitReader(zr, 1<<30)); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if r, err := maxminddb.Open(tmp); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("the downloaded file is not a database: %w", err)
	} else {
		r.Close()
	}
	return os.Rename(tmp, file)
}

// State is what the lookups are doing, in words.
func (d *DB) State() string { return d.state.Load().(string) }

// Lookup finds an address; the zero Place when it cannot.
func (d *DB) Lookup(ip string) Place {
	r := d.r.Load()
	addr := net.ParseIP(ip)
	if r == nil || addr == nil || addr.IsPrivate() || addr.IsLoopback() {
		return Place{}
	}
	var rec record
	if err := r.Lookup(addr, &rec); err != nil {
		return Place{}
	}
	p := Place{Country: rec.Country.ISOCode, City: rec.City.Names["en"]}
	if len(rec.Subdivisions) > 0 {
		p.Region = rec.Subdivisions[0].Names["en"]
	}
	if p.Country == "ZZ" {
		p.Country = ""
	}
	return p
}
