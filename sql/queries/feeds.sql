-- 1. CreateFeed (Required by handlerAddFeed)
-- name: CreateFeed :one
INSERT INTO feeds (id, created_at, updated_at, name, url, user_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- 2. GetFeedByUrl (Required by handlerFollow and handlerAddFeed)
-- name: GetFeedByUrl :one
SELECT id, created_at, updated_at, name, url, user_id
FROM feeds
WHERE url = $1;

-- 3. CreateFeedFollow (Required by handlerFollow and handlerAddFeed)
-- name: CreateFeedFollow :one
WITH inserted_feed_follow AS (
    INSERT INTO feed_follows (id, created_at, updated_at, user_id, feed_id)
    VALUES (sqlc.arg(id), sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(user_id), sqlc.arg(feed_id))
    -- This explicit list avoids the "star expansion" error
    RETURNING id, created_at, updated_at, user_id, feed_id
)
SELECT
    inserted_feed_follow.id,
    inserted_feed_follow.created_at,
    inserted_feed_follow.updated_at,
    inserted_feed_follow.user_id,
    inserted_feed_follow.feed_id,
    feeds.name AS feed_name,
    users.name AS user_name
FROM
    inserted_feed_follow
JOIN
    feeds ON inserted_feed_follow.feed_id = feeds.id
JOIN
    users ON inserted_feed_follow.user_id = users.id;

-- 4. GetFeedFollowsForUser (Required by handlerFollowing)
-- name: GetFeedFollowsForUser :many
SELECT
    ff.id,
    ff.created_at,
    ff.updated_at,
    ff.user_id,
    ff.feed_id,
    f.name AS feed_name,
    u.name AS user_name
FROM
    feed_follows ff
JOIN
    feeds f ON ff.feed_id = f.id
JOIN
    users u ON ff.user_id = u.id
WHERE
    ff.user_id = $1
ORDER BY
    f.name ASC;

-- 5. GetFeedsWithUserName (Likely required by handlerListFeeds)
-- name: GetFeedsWithUserName :many
SELECT
    f.id,
    f.created_at,
    f.updated_at,
    f.name,
    f.url,
    f.user_id,
    u.name AS user_name
FROM
    feeds f
JOIN
    users u ON f.user_id = u.id
ORDER BY
    f.name ASC;
