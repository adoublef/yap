package internal

import (
	"context"
	"database/sql"

	"github.com/adoublef/yap"
	"github.com/rs/xid"
)

func listYaps(ctx context.Context, db *sql.DB) (yy []*yap.Yap, err error) {
	var qry = `
SELECT
	y.id,
	y.content,
	y.region,
	SUM(CASE
		WHEN v.score = 0 THEN -1 
		WHEN v.score = 1 THEN 1
		ELSE 0 
	END) AS score
FROM
	yaps AS y
LEFT JOIN
	votes AS v ON y.id = v.yap
GROUP BY
	y.id, y.content
		`
	rs, err := db.QueryContext(ctx, qry)
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	for rs.Next() {
		var y yap.Yap
		err = rs.Scan(&y.ID, &y.Content, &y.Region, &y.Score)
		if err != nil {
			return nil, err
		}
		yy = append(yy, &y)
	}
	return yy, rs.Err()
}

func postYap(ctx context.Context, db *sql.DB, y *yap.Yap) (err error) {
	var qry = `
INSERT INTO yaps (id, content, region)
VALUES (?, ?, ?)
		`
	_, err = db.ExecContext(ctx, qry, y.ID, y.Content, y.Region)
	return
}

func makeVote(ctx context.Context, db *sql.DB, yap xid.ID, upvote bool) (err error) {
	var qry = `
INSERT INTO votes (yap, score)
VALUES (?, ?)
		`
	_, err = db.ExecContext(ctx, qry, yap, upvote)
	return
}

func yapFromID(ctx context.Context, db *sql.DB, yid xid.ID) (y *yap.Yap, err error) {
	const qry = `
SELECT
	y.id,
	y.content,
	y.region,
	SUM(CASE
		WHEN v.score = 0 THEN -1 
		WHEN v.score = 1 THEN 1
		ELSE 0 
	END) AS score
FROM
	yaps AS y
LEFT JOIN
	votes AS v ON y.id = v.yap
GROUP BY
	y.id, y.content
WHERE
	y.id = ?
		`
	return
}

func currentScore(ctx context.Context, db *sql.DB, yid xid.ID) (n int, err error) {
	var qry = `
SELECT 
	SUM(CASE
		WHEN v.score = 0 THEN -1 
		WHEN v.score = 1 THEN 1
		ELSE 0 
	END) AS score
FROM
	votes AS v
WHERE
	v.yap = ?
		`
	err = db.QueryRowContext(ctx, qry).Scan(&n)
	return
}
