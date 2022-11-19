package rss

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ToPoll struct {
	DBAccountID  	string
	DBUsername  	string
	Url  					string
	LastTweet  		time.Time
}

func (n *rssTooter) GetAccountsToPoll(ctx context.Context) ([]*ToPoll, error) {
	toPoll := make([]*ToPoll, 0)

	err := n.state.DB.DB().NewSelect().
		ColumnExpr("accounts.id AS db_account_id").
		ColumnExpr("accounts.username AS db_username").
		ColumnExpr("accounts.url AS url").
		ColumnExpr("max(statuses.created_at) AS last_tweet").
		TableExpr("accounts").
		Join("LEFT JOIN statuses on accounts.id = statuses.account_id").
		Where("actor_type = 'Person'").
		Where("bot = true").
		Where("domain IS NULL").
		GroupExpr("accounts.id").
		GroupExpr("accounts.username").
		GroupExpr("accounts.url").
		Scan(ctx, &toPoll)

	return toPoll, err
}

type IdDB interface {
  int64 | string
}

func ToIdDB[T IdDB](id T) string {
	idC := any(id)
	switch idC.(type) {
		case int64: return fmt.Sprintf("%026s", strconv.FormatInt(idC.(int64), 10))
		default: return fmt.Sprintf("%026s", id)
	}
}

func FromIdDB(id string) string {
	return strings.TrimLeft(id, "0")
}

func TolUsernameDB(username string) string {
	return strings.ToLower(username)
}
