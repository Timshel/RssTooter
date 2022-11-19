package rss

import (
   "sort"
   "time"

   "github.com/mmcdole/gofeed"
   "github.com/superseriousbusiness/gotosocial/internal/gtsmodel"
   "github.com/superseriousbusiness/gotosocial/internal/log"
)

type ToCreate struct {
   Account   *gtsmodel.Account
   Item      *gofeed.Item
}


func (n *rssTooter) refresh() {
   log.Infof(nil, "Initiate polling every %d minutes", n.pollFrequency)
   ticker := time.NewTicker(time.Duration(n.pollFrequency) * time.Minute)

   for {
      select {
         case <-n.ctx.Done(): return
         case <-ticker.C:
            var toCreate []ToCreate

            toPoll, err := n.GetAccountsToPoll(n.ctx)
            log.Infof(nil, "Started polling %d accounts", len(toPoll))
            if( err != nil ) {
               log.Errorf(nil, "Failed to retrieve accounts to poll: %s", err)
            }

            for _, infos := range toPoll {
               fp := gofeed.NewParser()
               feed, err := fp.ParseURL(infos.Url)
               if err != nil {
                  log.Errorf(nil, "Invalid feed url: %s", err)
                  continue
               }

               account, err := n.state.DB.GetAccountByID(n.ctx, infos.DBAccountID)
               if( err != nil ) {
                  log.Errorf(nil, "Failed to retrieve account: %s", err)
                  continue
               }

               for _, item := range feed.Items {
                  if( item.PublishedParsed.After(infos.LastTweet) ){
                     toCreate = append(toCreate, ToCreate { Account: account, Item: item })
                  }
               }
            }

            sort.SliceStable(toCreate, func(i, j int) bool {
               return toCreate[i].Item.PublishedParsed.Before(*toCreate[j].Item.PublishedParsed)
            })

            for _, create := range toCreate {
               err = n.PutStatus(n.ctx, &create)
               if( err != nil ) {
                  log.Errorf(nil, "Failed to create tweet %s: %s", create.Item, err)
               }
            }
      }
   }
}
