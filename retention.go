package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"log"
	"os"
	"time"

	routing "github.com/qiangxue/fasthttp-routing"
)

// Logs contain operation names only: database errors may contain connection
// strings, SQL arguments, or user data.
func retentionFailure(c *routing.Context, operation string) error {
	log.Printf("retention: %s failed", operation)
	writeJSONError(c, 500, "retention data unavailable")
	return nil
}

// Authenticated application requests and successful logins are return activity.
// The background Steam poller never calls this. One row per user per UTC date.
func recordReturnActivity(userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if db == nil {
		log.Print("retention: activity write unavailable")
		return
	}
	_, err := db.Exec(ctx, `INSERT INTO user_return_activity (user_id, activity_date)
 VALUES ($1, $2) ON CONFLICT DO NOTHING`, userID, time.Now().UTC().Format("2006-01-02"))
	if err != nil {
		log.Print("retention: activity write failed")
	}
}

type cohortMetric struct {
	Scope       string   `json:"scope"`
	Source      string   `json:"source"`
	Day         int      `json:"day"`
	Eligible    int      `json:"eligible_users"`
	Returning   int      `json:"returning_users"`
	Immature    int      `json:"immature_users"`
	Unavailable int      `json:"historical_unavailable_users"`
	Rate        *float64 `json:"rate"`
}

// Exact-day retention; target UTC dates must be complete. Pre-instrumentation
// signups are conservatively unavailable, never treated as zero retention.
const retentionSQL = `
WITH cohorts AS (
 SELECT u.id, CASE WHEN signup_source IN ('direct','reddit_patientgamers','backloggd')
 THEN signup_source ELSE 'unknown' END AS source,
 (created_at AT TIME ZONE 'UTC')::date AS signup_date, created_at >= $2 AS observable
 FROM users u
), observations AS (
 SELECT c.*, d.day, signup_date + d.day < $1::date AS mature,
 a.user_id IS NOT NULL AS returned
 FROM cohorts c CROSS JOIN (VALUES (1),(7),(30)) d(day)
 LEFT JOIN user_return_activity a ON a.user_id = c.id AND a.activity_date = signup_date + d.day
)
SELECT CASE WHEN GROUPING(source) = 1 THEN 'overall' ELSE 'source' END,
 COALESCE(source, ''), day,
 COUNT(*) FILTER (WHERE mature AND observable),
 COUNT(*) FILTER (WHERE mature AND observable AND returned),
 COUNT(*) FILTER (WHERE NOT mature),
 COUNT(*) FILTER (WHERE mature AND NOT observable)
FROM observations GROUP BY GROUPING SETS ((day), (source, day))
ORDER BY GROUPING(source) DESC, source, day`

// GET /api/admin/retention requires Authorization: Bearer <ADMIN_KEY>.
// Query-string credentials are rejected, even with a valid header.
func getRetentionStats(c *routing.Context) error {
	c.Response.Header.Set("Cache-Control", "no-store")
	key := os.Getenv("ADMIN_KEY")
	expected := sha256.Sum256([]byte("Bearer " + key))
	actual := sha256.Sum256(c.Request.Header.Peek("Authorization"))
	if key == "" || c.QueryArgs().Len() != 0 || subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
		writeJSONError(c, 401, "unauthorized")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	now := time.Now().UTC()
	var started time.Time
	if db == nil {
		return retentionFailure(c, "database unavailable")
	}
	if err := db.QueryRow(ctx, `SELECT started_at FROM retention_tracking WHERE singleton = TRUE`).Scan(&started); err != nil {
		return retentionFailure(c, "tracking start query")
	}
	rows, err := db.Query(ctx, retentionSQL, now.Format("2006-01-02"), started)
	if err != nil {
		return retentionFailure(c, "cohort query")
	}
	defer rows.Close()
	metrics := []cohortMetric{}
	for rows.Next() {
		var m cohortMetric
		if err := rows.Scan(&m.Scope, &m.Source, &m.Day, &m.Eligible, &m.Returning, &m.Immature, &m.Unavailable); err != nil {
			return retentionFailure(c, "cohort scan")
		}
		if m.Eligible > 0 {
			rate := float64(m.Returning) / float64(m.Eligible)
			m.Rate = &rate
		}
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return retentionFailure(c, "cohort iteration")
	}
	// Empty databases still expose all requested overall horizons with null rates.
	if len(metrics) == 0 {
		for _, day := range []int{1, 7, 30} {
			metrics = append(metrics, cohortMetric{Scope: "overall", Day: day})
		}
	}
	out, err := json.Marshal(map[string]any{"as_of": now, "tracking_started_at": started, "cohorts": metrics})
	if err != nil {
		return retentionFailure(c, "response encoding")
	}
	c.SetContentType("application/json")
	c.Write(out)
	return nil
}
