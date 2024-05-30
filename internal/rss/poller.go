package rss

import (
   "compress/flate"
   "compress/gzip"
   "context"
   "fmt"
   "net/http"
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

            fp := gofeed.NewParser()
            client := http.Client{Timeout: time.Duration(30) * time.Second}

            for _, infos := range toPoll {
               account, err := n.state.DB.GetAccountByID(n.ctx, infos.DBAccountID)
               if( err != nil ) {
                  log.Errorf(nil, "Failed to retrieve account: %s", err)
                  continue
               }

               etag := ""
               if len(account.Fields) > 0 && account.Fields[0].Name == "etag" {
                  etag = account.Fields[0].Value
               }

               feed, err := parseURLWithCache(fp, &client, infos.Url, etag, &account.FetchedAt, n.ctx)
               if err != nil {
                  log.Errorf(nil, "Invalid feed url: %s", err)
                  continue
               }

               if feed.Feed != nil {
                  size := len(toCreate)
                  for _, item := range feed.Feed.Items {
                     if( item.PublishedParsed.After(infos.LastTweet) ){
                        toCreate = append(toCreate, ToCreate { Account: account, Item: item })
                     }
                  }
                  if len(feed.Feed.Items) > 0 && len(toCreate) == size {
                     log.Warnf(nil, "Feed was not cached but returned no new items :( (%s)", infos.Url)
                  }
               }

               account.FetchedAt = *feed.LastModified
               account.Fields = []*gtsmodel.Field{
                  &gtsmodel.Field { Name: "etag", Value: feed.Etag, },
               }

               err = n.state.DB.UpdateAccount(n.ctx, account, "fields", "fetched_at")
               if err != nil {
                  log.Errorf(nil, "Failed to save modified account: %s", err)
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

type HTTPFeed struct {
   Feed              *gofeed.Feed
   Etag              string
   LastModified      *time.Time
}

func parseURLWithCache(f *gofeed.Parser, client *http.Client, feedURL string, etag string, lastModified *time.Time, ctx context.Context) (feed *HTTPFeed, err error) {
   location := time.FixedZone("GMT", 0)

   req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
   if err != nil {
      return nil, err
   }
   req.Header.Set("User-Agent", f.UserAgent)
   req.Header.Set("Accept-Encoding", "gzip, deflate")

   if etag != "" {
      req.Header.Set("If-None-Match", etag)
   }

   if lastModified != nil {
      req.Header.Set("If-Modified-Since", lastModified.In(location).Format(time.RFC1123))
   }

   if f.AuthConfig != nil && f.AuthConfig.Username != "" && f.AuthConfig.Password != "" {
      req.SetBasicAuth(f.AuthConfig.Username, f.AuthConfig.Password)
   }

   resp, err := client.Do(req)

   if err != nil {
      return nil, err
   }

   if resp.StatusCode != 200 && resp.StatusCode != 206 && resp.StatusCode != 304 {
      return nil, fmt.Errorf("Invalid returned HTTPCode: %s - %s", resp.StatusCode, resp.Status)
   }

   httpFeed := HTTPFeed {
      Etag: resp.Header.Get("Etag"),
      LastModified: lastModified,
   }

   if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
      parsed, err := time.ParseInLocation(time.RFC1123, lastModified, location)
      if err == nil {
         httpFeed.LastModified = &parsed
      }
   }

   if resp.StatusCode == 304 {
      return &httpFeed, nil
   }

   reader := resp.Body

   if !resp.Uncompressed {
      switch ce := resp.Header.Get("Content-Encoding"); ce {
      case "gzip":
         reader, err = gzip.NewReader(reader)
         if err != nil {
            return nil, fmt.Errorf("Failed to initialize gzip reader", err)
         }
      case "deflate":
         reader = flate.NewReader(reader)
      case "":
         break // Decompression was handled by transport
      default:
         return nil, fmt.Errorf("Unknow Content-Encoding: %s", ce)
      }
   }

   defer func() {
      ce := reader.Close()
      if ce != nil {
         err = ce
      }
   }()

   res, err := f.Parse(reader)
   if err != nil {
      return nil, err
   }
   httpFeed.Feed = res

   return &httpFeed, nil
}
