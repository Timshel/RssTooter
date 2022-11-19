package rss

import (
   "context"
   "crypto/rsa"
   "crypto/rand"
   "fmt"
   "net/url"
   "regexp"
   "time"

   "codeberg.org/gruf/go-kv"
   "github.com/antchfx/htmlquery"
   "github.com/cespare/xxhash"
   "github.com/mmcdole/gofeed"
   "github.com/superseriousbusiness/gotosocial/internal/ap"
   "github.com/superseriousbusiness/gotosocial/internal/config"
   "github.com/superseriousbusiness/gotosocial/internal/gtserror"
   "github.com/superseriousbusiness/gotosocial/internal/gtsmodel"
   "github.com/superseriousbusiness/gotosocial/internal/id"
   "github.com/superseriousbusiness/gotosocial/internal/log"
   "github.com/superseriousbusiness/gotosocial/internal/uris"
   "golang.org/x/crypto/bcrypt"
)

func generateAccount(url *url.URL, dbUsername string) (*gtsmodel.Account, error) {
   l := log.WithFields(kv.Fields{{K: "url", V: url.String()},}...)

   doc, err := htmlquery.LoadURL(url.String())
   if err != nil {
      fmt.Println("Error:", err)
      return nil, err
   }

   atomNode := htmlquery.FindOne(doc, "//link[contains(@type,'application/atom+xml')]")
   atomPath := htmlquery.SelectAttr(atomNode, "href")

   fp := gofeed.NewParser()
   feedUrl := fmt.Sprintf("https://%s%s", url.Hostname(), atomPath)
   feed, err := fp.ParseURL(feedUrl)
   if err != nil {
      l.Errorf("Can't find a valid rss feed: %s", err)
      return nil, err
   }

   iconUrl := ""
   if feed.Image != nil {
      iconUrl = feed.Image.URL
   } else {
      iconNode := htmlquery.FindOne(doc, "//link[@rel='icon']")
      if iconNode != nil {
         iconPath := htmlquery.SelectAttr(iconNode, "href")
         if( len(iconPath) > 1 ){
            iconUrl = fmt.Sprintf("https://%s%s", url.Hostname(), iconPath)
         }
      }
   }

   key, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
   if err != nil {
      l.Errorf("error creating new rsa key: %s", err)
      return nil, err
   }

   // if we have db.ErrNoEntries, we just don't have an
   // account yet so create one before we proceed
   accountURIs := uris.GenerateURIsForAccount(dbUsername)
   acct := &gtsmodel.Account{
      ID:                    id.NewULID(),
      Username:              dbUsername,
      DisplayName:           feed.Title,
      Note:                  feed.Description,
      Bot:                   &[]bool{true}[0],
      Locked:                &[]bool{false}[0],
      Discoverable:          &[]bool{true}[0],
      URL:                   feedUrl,
      PrivateKey:            key,
      PublicKey:             &key.PublicKey,
      PublicKeyURI:          accountURIs.PublicKeyURI,
      ActorType:             ap.ActorPerson,
      URI:                   accountURIs.UserURI,
      AvatarRemoteURL:       iconUrl,
      InboxURI:              accountURIs.InboxURI,
      OutboxURI:             accountURIs.OutboxURI,
      FollowersURI:          accountURIs.FollowersURI,
      FollowingURI:          accountURIs.FollowingURI,
      FeaturedCollectionURI: accountURIs.FeaturedCollectionURI,
   }

   return acct, nil
}

func (n *rssTooter) NewUser(ctx context.Context, resource string) (string, error) {
   l := log.WithFields(kv.Fields{{K: "resource", V: resource},}...)

   hostRg := regexp.MustCompile(fmt.Sprintf("@%s$", config.GetHost()))
   cleaned := hostRg.ReplaceAllString(resource, ``)
   l.Infof("Cleaned resource: %s", cleaned)

   url, err := url.Parse(fmt.Sprintf("https://%s", cleaned))
   if err != nil {
      l.Errorf("Can't find a valid rss feed: %s", err)
      return "", err
   }

   pathQuery := fmt.Sprintf("%s?%s", url.Path, url.RawQuery)
   var dbUsername string
   if( len(pathQuery) > 1 ){
      dbUsername = TolUsernameDB(fmt.Sprintf("%s.%d", url.Hostname(), xxhash.Sum64String(pathQuery)))
   } else {
      dbUsername = TolUsernameDB(url.Hostname())
   }
   l.Errorf("Init for user: %s", dbUsername)

   available, err := n.state.DB.IsUsernameAvailable(ctx, dbUsername)
   if available && err == nil {
      // Pre-fetch a transport for requesting username, used by later dereferencing.
      tsport, err := n.transportController.NewTransportForUsername(ctx, "")
      if err != nil {
         return "", gtserror.Newf("couldn't create transport: %w", err)
      }

      acct, err := generateAccount(url, dbUsername)

      err = n.dereferencer.FetchRemoteAccountAvatar(ctx, tsport, acct)
      if err != nil {
         return "", fmt.Errorf("Error fetching account (%s) media: %s", dbUsername, err)
      }

      // insert the new account!
      if err := n.state.DB.PutAccount(ctx, acct); err != nil {
         return "", err
      }

      pw, err := bcrypt.GenerateFromPassword([]byte(n.userPassword), bcrypt.DefaultCost)
      if err != nil {
         return "", fmt.Errorf("error hashing password: %s", err)
      }

      u := &gtsmodel.User{
         ID:                     acct.ID,
         AccountID:              acct.ID,
         Account:                acct,
         EncryptedPassword:      string(pw),
         Email:                  dbUsername + "@rss.tooter.com",
         ConfirmedAt:            time.Now(),
         Approved:               &[]bool{true}[0],
      }

      // insert the user!
      return dbUsername, n.state.DB.PutUser(ctx, u)
   }

   return dbUsername, err
}
