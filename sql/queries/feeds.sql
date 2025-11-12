-- name: CreateFeed :one
INSERT INTO feeds (id, created_at, updated_at, name, url, user_id)
VALUES (@id, @created_at, @updated_at, @name, @url, @user_id)
RETURNING *;

-- name: GetFeedByUrl :one
SELECT * FROM feeds
WHERE url = @url;

-- name: GetFeedsWithUserName :many
SELECT
    feeds.*,
    users.name AS user_name
FROM feeds
JOIN users ON feeds.user_id = users.id
ORDER BY feeds.created_at DESC;

-- name: CreateFeedFollow :one
INSERT INTO feed_follows (user_id, feed_id)
VALUES (@user_id, @feed_id)
RETURNING *;

-- name: GetFeedFollowForUserAndFeed :one
SELECT
    feed_follows.*,
    users.name AS user_name,
    feeds.name AS feed_name
FROM feed_follows
JOIN users ON feed_follows.user_id = users.id
JOIN feeds ON feed_follows.feed_id = feeds.id
WHERE feed_follows.user_id = @user_id
  AND feed_follows.feed_id = @feed_id;

-- name: GetFeedFollowsForUser :many
SELECT
    feed_follows.*,
    feeds.name AS feed_name
FROM feed_follows
JOIN feeds ON feed_follows.feed_id = feeds.id
WHERE feed_follows.user_id = @user_id;

-- name: DeleteFeedFollow :exec
DELETE FROM feed_follows
WHERE feed_follows.user_id = @user_id
  AND feed_follows.feed_id = @feed_id;
