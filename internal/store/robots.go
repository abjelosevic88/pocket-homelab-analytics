package store

import (
	"context"
	"strings"
)

// RobotHit is how often a robot read a page of a site in one hour.
type RobotHit struct {
	SiteID int64
	TS     int64 // the hour's start
	Name   string
	Kind   string
	Path   string
	Hits   int
}

// AddRobotHits adds to the robots' counts.
func (s *Store) AddRobotHits(ctx context.Context, hits []RobotHit) error {
	if len(hits) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, h := range hits {
		if _, err := tx.ExecContext(ctx, `INSERT INTO robots (site_id, ts, name, kind, path, hits) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(site_id, ts, name, path) DO UPDATE SET hits = hits + excluded.hits`, h.SiteID, h.TS, h.Name, h.Kind, h.Path, h.Hits); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RobotRow is one line of a robots list: a robot, a kind or a page.
type RobotRow struct {
	Value string `json:"value"`
	Kind  string `json:"kind,omitempty"`
	Hits  int    `json:"hits"`            // pages read
	Pages int    `json:"pages,omitempty"` // different pages (by robot) or robots (by kind, by page)
	Last  int64  `json:"last,omitempty"`  // the hour it last came
}

// RobotQuery narrows the robots of a site in [From, To).
type RobotQuery struct {
	SiteID   int64
	From, To int64
	By       string   // name | kind | path
	Kinds    []string // only these kinds
	Name     string   // only this robot
	Limit    int
}

// Robots lists who read the site, most first.
func (s *Store) Robots(ctx context.Context, q RobotQuery) ([]RobotRow, int, error) {
	if q.Limit <= 0 || q.Limit > 1000 {
		q.Limit = 10
	}
	where, args := "site_id = ? AND ts >= ? AND ts < ?", []any{q.SiteID, q.From, q.To}
	if len(q.Kinds) > 0 {
		where += " AND kind IN (?" + strings.Repeat(", ?", len(q.Kinds)-1) + ")"
		for _, k := range q.Kinds {
			args = append(args, k)
		}
	}
	if q.Name != "" {
		where += " AND name = ?"
		args = append(args, q.Name)
	}
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(hits), 0) FROM robots WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	sel := "name, MAX(kind), SUM(hits), COUNT(DISTINCT path), MAX(ts)"
	switch q.By {
	case "kind":
		sel = "kind, kind, SUM(hits), COUNT(DISTINCT name), MAX(ts)"
	case "path":
		sel = "path, '', SUM(hits), COUNT(DISTINCT name), MAX(ts)"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+sel+` FROM robots WHERE `+where+` GROUP BY 1 ORDER BY 3 DESC, 1 LIMIT ?`, append(args, q.Limit)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []RobotRow{}
	for rows.Next() {
		var r RobotRow
		if err := rows.Scan(&r.Value, &r.Kind, &r.Hits, &r.Pages, &r.Last); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// HasRobots reports whether any robot was ever seen on a site.
func (s *Store) HasRobots(ctx context.Context, siteID int64) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT 1 FROM robots WHERE site_id = ? LIMIT 1`, siteID).Scan(&n)
	return n == 1
}
