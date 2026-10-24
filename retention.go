package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	routing "github.com/qiangxue/fasthttp-routing"
)

// getRetentionStats handles GET /api/admin/retention.
// Requires ?key=<ADMIN_KEY>. Returns signup counts and return-visit rates
// broken down by day, week, month, and signup source.
func getRetentionStats(c *routing.Context) error {
	adminKey := os.Getenv("ADMIN_KEY")
	if adminKey == "" || string(c.QueryArgs().Peek("key")) != adminKey {
		c.SetStatusCode(401)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"unauthorized"}`)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	now := time.Now().UTC()
	day7ago := now.Add(-7 * 24 * time.Hour)
	day30ago := now.Add(-30 * 24 * time.Hour)

	var totalUsers int
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&totalUsers)

	var newUsers7d, newUsers30d int
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE created_at >= $1`, day7ago).Scan(&newUsers7d)
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE created_at >= $1`, day30ago).Scan(&newUsers30d)

	// Active = logged in at least once after signup (returned user).
	var activeUsers7d, activeUsers30d int
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE last_login_at >= $1`, day7ago).Scan(&activeUsers7d)
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE last_login_at >= $1`, day30ago).Scan(&activeUsers30d)

	type sourceRow struct {
		Source string `json:"source"`
		Count  int    `json:"count"`
	}
	var sources []sourceRow
	rows, err := db.Query(ctx, `
		SELECT COALESCE(NULLIF(signup_source, ''), 'direct') AS source, COUNT(*) AS cnt
		FROM users
		GROUP BY source
		ORDER BY cnt DESC
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var r sourceRow
			_ = rows.Scan(&r.Source, &r.Count)
			sources = append(sources, r)
		}
	}
	if sources == nil {
		sources = []sourceRow{}
	}

	c.SetContentType("application/json")
	out, _ := json.Marshal(map[string]interface{}{
		"as_of":             now.Format(time.RFC3339),
		"total_users":       totalUsers,
		"new_users_7d":      newUsers7d,
		"new_users_30d":     newUsers30d,
		"active_users_7d":   activeUsers7d,
		"active_users_30d":  activeUsers30d,
		"signups_by_source": sources,
	})
	c.Write(out)
	return nil
}
