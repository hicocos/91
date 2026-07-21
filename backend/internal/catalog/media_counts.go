package catalog

import "context"

type DriveMediaCounts struct {
	Video int
	Audio int
}

func (c *Catalog) CountMediaByDrive(ctx context.Context) (map[string]DriveMediaCounts, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT drive_id,
		       COUNT(CASE WHEN COALESCE(media_type, 'video') = 'video' THEN 1 END),
		       COUNT(CASE WHEN media_type = 'audio' THEN 1 END)
		  FROM videos
		 WHERE COALESCE(hidden, 0) = 0
		 GROUP BY drive_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]DriveMediaCounts)
	for rows.Next() {
		var id string
		var counts DriveMediaCounts
		if err := rows.Scan(&id, &counts.Video, &counts.Audio); err != nil {
			return nil, err
		}
		out[id] = counts
	}
	return out, rows.Err()
}
