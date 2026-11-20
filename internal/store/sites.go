package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

const siteCols = `id, user_id, domain, name, tz, also, options, proxy_key, position, first_hit, last_hit, created_at, updated_at`

func scanSite(row interface{ Scan(...any) error }) (*Site, error) {
	var s Site
	var also, opts string
	if err := row.Scan(&s.ID, &s.UserID, &s.Domain, &s.Name, &s.TZ, &also, &opts, &s.ProxyKey, &s.Position, &s.FirstHit, &s.LastHit, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	s.Also = strings.Fields(also)
	s.Options = DefaultOptions()
	_ = json.Unmarshal([]byte(opts), &s.Options)
	if s.Options.Exclude == nil {
		s.Options.Exclude = []string{}
	}
	if s.Also == nil {
		s.Also = []string{}
	}
	return &s, nil
}

func (s *Store) sites(ctx context.Context, q string, args ...any) ([]*Site, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+siteCols+` FROM sites `+q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Site{}
	for rows.Next() {
		st, err := scanSite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// ListSites is one person's sites in their order.
func (s *Store) ListSites(ctx context.Context, uid int64) ([]*Site, error) {
	return s.sites(ctx, `WHERE user_id = ? ORDER BY position, domain`, uid)
}

// AllSites is every site on the box (the collector's domain table, background work).
func (s *Store) AllSites(ctx context.Context) ([]*Site, error) {
	return s.sites(ctx, `ORDER BY id`)
}

// GetSite is one of the person's sites (someone else's = not found).
func (s *Store) GetSite(ctx context.Context, uid, id int64) (*Site, error) {
	return scanSite(s.DB.QueryRowContext(ctx, `SELECT `+siteCols+` FROM sites WHERE id = ? AND user_id = ?`, id, uid))
}

// SiteByDomain finds a site by its domain, for anyone.
func (s *Store) SiteByDomain(ctx context.Context, domain string) (*Site, error) {
	return scanSite(s.DB.QueryRowContext(ctx, `SELECT `+siteCols+` FROM sites WHERE domain = ?`, domain))
}

// CreateSite adds a site; the domain must be free on the box.
func (s *Store) CreateSite(ctx context.Context, st *Site) error {
	if st.Options.Exclude == nil {
		st.Options.Exclude = []string{}
	}
	var pos int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) + 1 FROM sites WHERE user_id = ?`, st.UserID).Scan(&pos)
	st.Position = pos
	res, err := s.DB.ExecContext(ctx, `INSERT INTO sites (user_id, domain, name, tz, also, options, proxy_key, position) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		st.UserID, st.Domain, st.Name, st.TZ, strings.Join(st.Also, " "), toJSON(st.Options), st.ProxyKey, st.Position)
	if err != nil {
		return conflict(err)
	}
	st.ID, _ = res.LastInsertId()
	got, err := s.GetSite(ctx, st.UserID, st.ID)
	if err == nil {
		*st = *got
	}
	return err
}

// UpdateSite saves a site's name, domain, zone and options.
func (s *Store) UpdateSite(ctx context.Context, st *Site) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE sites SET domain = ?, name = ?, tz = ?, also = ?, options = ?, proxy_key = ?, position = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		st.Domain, st.Name, st.TZ, strings.Join(st.Also, " "), toJSON(st.Options), st.ProxyKey, st.Position, Now(), st.ID, st.UserID)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSite removes a site with everything counted for it.
func (s *Store) DeleteSite(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM sites WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ResetSite forgets every visit of a site (the site and its goals stay).
func (s *Store) ResetSite(ctx context.Context, uid, id int64) error {
	if _, err := s.GetSite(ctx, uid, id); err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM hits WHERE site_id = ?`, `DELETE FROM visits WHERE site_id = ?`, `UPDATE sites SET first_hit = 0, last_hit = 0 WHERE id = ?`} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// OrderSites saves the person's order of their sites.
func (s *Store) OrderSites(ctx context.Context, uid int64, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE sites SET position = ? WHERE id = ? AND user_id = ?`, i+1, id, uid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// TrimSite removes a site's visits that started before the cut (Unix seconds).
func (s *Store) TrimSite(ctx context.Context, siteID, before int64) (int64, error) {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM hits WHERE site_id = ? AND ts < ?`, siteID, before); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM visits WHERE site_id = ? AND started < ?`, siteID, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---- goals ----

func (s *Store) ListGoals(ctx context.Context, uid, siteID int64) ([]Goal, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, site_id, name, kind, match, created_at FROM goals WHERE user_id = ? AND site_id = ? ORDER BY id`, uid, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Goal{}
	for rows.Next() {
		var g Goal
		if err := rows.Scan(&g.ID, &g.SiteID, &g.Name, &g.Kind, &g.Match, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) GetGoal(ctx context.Context, uid, id int64) (*Goal, error) {
	var g Goal
	err := s.DB.QueryRowContext(ctx, `SELECT id, site_id, name, kind, match, created_at FROM goals WHERE id = ? AND user_id = ?`, id, uid).Scan(&g.ID, &g.SiteID, &g.Name, &g.Kind, &g.Match, &g.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	return &g, nil
}

func (s *Store) CreateGoal(ctx context.Context, uid int64, g *Goal) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO goals (user_id, site_id, name, kind, match) VALUES (?, ?, ?, ?, ?)`, uid, g.SiteID, g.Name, g.Kind, g.Match)
	if err != nil {
		return err
	}
	g.ID, _ = res.LastInsertId()
	got, err := s.GetGoal(ctx, uid, g.ID)
	if err == nil {
		*g = *got
	}
	return err
}

func (s *Store) UpdateGoal(ctx context.Context, uid int64, g *Goal) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE goals SET name = ?, kind = ?, match = ? WHERE id = ? AND user_id = ?`, g.Name, g.Kind, g.Match, g.ID, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteGoal(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM goals WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- funnels ----

func scanFunnel(row interface{ Scan(...any) error }) (*Funnel, error) {
	var f Funnel
	var steps string
	if err := row.Scan(&f.ID, &f.SiteID, &f.Name, &steps, &f.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	_ = json.Unmarshal([]byte(steps), &f.Steps)
	if f.Steps == nil {
		f.Steps = []Step{}
	}
	return &f, nil
}

func (s *Store) ListFunnels(ctx context.Context, uid, siteID int64) ([]*Funnel, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, site_id, name, steps, created_at FROM funnels WHERE user_id = ? AND site_id = ? ORDER BY id`, uid, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Funnel{}
	for rows.Next() {
		f, err := scanFunnel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) GetFunnel(ctx context.Context, uid, id int64) (*Funnel, error) {
	return scanFunnel(s.DB.QueryRowContext(ctx, `SELECT id, site_id, name, steps, created_at FROM funnels WHERE id = ? AND user_id = ?`, id, uid))
}

func (s *Store) CreateFunnel(ctx context.Context, uid int64, f *Funnel) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO funnels (user_id, site_id, name, steps) VALUES (?, ?, ?, ?)`, uid, f.SiteID, f.Name, toJSON(f.Steps))
	if err != nil {
		return err
	}
	f.ID, _ = res.LastInsertId()
	got, err := s.GetFunnel(ctx, uid, f.ID)
	if err == nil {
		*f = *got
	}
	return err
}

func (s *Store) UpdateFunnel(ctx context.Context, uid int64, f *Funnel) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE funnels SET name = ?, steps = ? WHERE id = ? AND user_id = ?`, f.Name, toJSON(f.Steps), f.ID, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteFunnel(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM funnels WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

var _ = sql.ErrNoRows
