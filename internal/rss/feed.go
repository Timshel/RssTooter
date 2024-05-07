package rss

import (
   "context"
   "fmt"
   netUrl "net/url"
   "regexp"
   "strings"

   "github.com/antchfx/htmlquery"
   "github.com/cespare/xxhash"
   "github.com/mmcdole/gofeed"
   "github.com/superseriousbusiness/gotosocial/internal/config"
   "github.com/superseriousbusiness/gotosocial/internal/state"
   "golang.org/x/net/html"
)

var hostRg        = regexp.MustCompile(fmt.Sprintf("@%s$", config.GetHost()))
var baseRg        = regexp.MustCompile(`/[^/\.]+\.xml|\.rss|\.atom$`)
var cleanHostRg   = regexp.MustCompile(`^www\.`)
var cleanPathRg   = regexp.MustCompile(`(/atom\.xml|/rss\.xml|\.xml|\.rss|\.atom)$`)
var mastoCharsRg  = regexp.MustCompile("[^a-z0-9_\\.]")
var iconRg        = regexp.MustCompile("(?i)icon")

// RssTooter just implements the RssTooter interface
type rssFeed struct {
   BaseUrl              *netUrl.URL
   Doc                  *html.Node
   FeedUrl              *netUrl.URL
   Feed                 *gofeed.Feed
   DbUsername           string
}

func NewRssFeed(state *state.State, ctx context.Context, resource string) (string, *rssFeed, error) {
   cleaned := hostRg.ReplaceAllString(resource, ``)

   if !strings.HasPrefix(cleaned, "http") {
      dbUsername := TolUsernameDB(cleaned)

      available, err := state.DB.IsUsernameAvailable(ctx, dbUsername)
      if !available {
         return dbUsername, nil, err
      }

      cleaned = fmt.Sprintf("https://%s", cleaned)
   }

   url, err := netUrl.Parse(cleaned)
   if err != nil {
      return "", nil, fmt.Errorf("Not a valid url %s: %s", cleaned, err)
   }

   var doc *html.Node
   fp := gofeed.NewParser()

   feedUrl := url
   baseUrl := url
   feed, err := fp.ParseURL(feedUrl.String())
   if err != nil {
      doc, err = htmlquery.LoadURL(url.String())
      if err != nil {
         return "", nil, fmt.Errorf("Failed to load HTML from %s: %s", url, err)
      }

      node := htmlquery.FindOne(doc, "//link[contains(@type,'application/atom+xml')]")
      if node == nil {
         node = htmlquery.FindOne(doc, "//link[contains(@type,'application/rss+xml')]")
      }
      if node == nil {
         return "", nil, fmt.Errorf("Can't find any feed on %s", url)
      }
      feedPath := htmlquery.SelectAttr(node, "href")

      feedUrlStr := fmt.Sprintf("https://%s%s", url.Hostname(), feedPath)
      feedUrl, err = netUrl.Parse(feedUrlStr)
      if err != nil {
         return "", nil, fmt.Errorf("Not a valid feed url %s: %s", feedUrlStr, err)
      }

      feed, err = fp.ParseURL(feedUrlStr)
      if err != nil {
         return "", nil, fmt.Errorf("Invalid feed at %s: %s", feedUrl, err)
      }
   } else {
      baseUrlStr := baseRg.ReplaceAllString(feed.Link, ``)

      baseUrl, err = netUrl.Parse(baseUrlStr)
      if err != nil {
         return "", nil, fmt.Errorf("Invalid resolved baseUrl %s: %s", baseUrl, err)
      }

      doc, err = htmlquery.LoadURL(baseUrl.String())
      if err != nil {
         return "", nil, fmt.Errorf("Failed to load HTML from %s: %s", url, err)
      }
   }

   var dbUsername = ""
   hostName := cleanHostRg.ReplaceAllString(feedUrl.Hostname(), ``)
   feedPath := cleanPathRg.ReplaceAllString(feedUrl.Path, ``)
   if len(feedPath) > 20 || len(feedUrl.RawQuery) > 0 {
      pathQuery := fmt.Sprintf("%s?%s", feedUrl.Path, feedUrl.RawQuery)
      dbUsername = TolUsernameDB(fmt.Sprintf("%s.%d", hostName, xxhash.Sum64String(pathQuery)))
   } else {
      dbUsername = TolUsernameDB( hostName + mastoCharsRg.ReplaceAllString(feedPath, `.`))
   }

   available, err := state.DB.IsUsernameAvailable(ctx, dbUsername)
   if !available {
      return dbUsername, nil, err
   }

   rssFeed := rssFeed{
      BaseUrl:          baseUrl,
      Doc:              doc,
      FeedUrl:          feedUrl,
      Feed:             feed,
      DbUsername:       dbUsername,
   }

   return "", &rssFeed, nil
}

func (r *rssFeed) ExtractDescription() string {
   description := r.Feed.Description

   if len(description) == 0 {
      descrNode := htmlquery.FindOne(r.Doc, "//head/meta[contains(@name,'description')]")
      if descrNode != nil {
         description = htmlquery.SelectAttr(descrNode, "content")
      }
   }

   return fmt.Sprintf("%s <br> Proxy account for: <a href='%s'>%s<a>", description, r.FeedUrl, r.FeedUrl)
}

func (r *rssFeed) ExtractIcon() string {
   iconUrl := ""

   if r.Feed.Image != nil {
      iconUrl = r.Feed.Image.URL
   } else {
      for _, iconNode := range htmlquery.Find(r.Doc, "//head/link[@type='image/png']") {
         rel := htmlquery.SelectAttr(iconNode, "rel")

         if iconRg.MatchString(rel) {
            iconPath := htmlquery.SelectAttr(iconNode, "href")
            if( len(iconPath) > 1 ){
               if strings.Contains(iconPath, "http") {
                  iconUrl = iconPath
               } else {
                  iconUrl = fmt.Sprintf("https://%s%s", r.BaseUrl.Hostname(), iconPath)
               }
               break
            }
         }
      }
      if len(iconUrl) == 0 {
         iconUrl = fmt.Sprintf("http://%s/assets/logo.png", config.GetHost())
      }
   }

   return iconUrl
}
