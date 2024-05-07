package rss

import (
   "context"
   "crypto/rsa"
   "crypto/rand"
   "fmt"
   "time"

   "github.com/superseriousbusiness/gotosocial/internal/ap"
   "github.com/superseriousbusiness/gotosocial/internal/gtserror"
   "github.com/superseriousbusiness/gotosocial/internal/gtsmodel"
   "github.com/superseriousbusiness/gotosocial/internal/id"
   "github.com/superseriousbusiness/gotosocial/internal/uris"
   "golang.org/x/crypto/bcrypt"
)

func (n *rssTooter) NewUser(ctx context.Context, resource string) (string, error) {
   alreadyExistName, rssFeed, err := NewRssFeed(n.state, ctx, resource)

   if len(alreadyExistName) == 0 && err == nil {
      // Pre-fetch a transport for requesting username, used by later dereferencing.
      tsport, err := n.transportController.NewTransportForUsername(ctx, "")
      if err != nil {
         return "", gtserror.Newf("couldn't create transport: %w", err)
      }

      key, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
      if err != nil {
         return "", fmt.Errorf("Error geenrating account keys: (%s)", err)
      }

      accountID := id.NewULID()
      settings := &gtsmodel.AccountSettings{
         AccountID: accountID,
         Privacy:   gtsmodel.VisibilityPublic,
      }

      // if we have db.ErrNoEntries, we just don't have an
      // account yet so create one before we proceed
      accountURIs := uris.GenerateURIsForAccount(rssFeed.DbUsername)
      acct := &gtsmodel.Account{
         ID:                    accountID,
         Username:              rssFeed.DbUsername,
         DisplayName:           rssFeed.Feed.Title,
         Note:                  rssFeed.ExtractDescription(),
         Bot:                   &[]bool{true}[0],
         Locked:                &[]bool{false}[0],
         Discoverable:          &[]bool{true}[0],
         URL:                   rssFeed.FeedUrl.String(),
         PrivateKey:            key,
         PublicKey:             &key.PublicKey,
         PublicKeyURI:          accountURIs.PublicKeyURI,
         ActorType:             ap.ActorPerson,
         URI:                   accountURIs.UserURI,
         AvatarRemoteURL:       rssFeed.ExtractIcon(),
         InboxURI:              accountURIs.InboxURI,
         OutboxURI:             accountURIs.OutboxURI,
         FollowersURI:          accountURIs.FollowersURI,
         FollowingURI:          accountURIs.FollowingURI,
         FeaturedCollectionURI: accountURIs.FeaturedCollectionURI,
         Settings:              settings,
      }

      err = n.dereferencer.FetchRemoteAccountAvatar(ctx, tsport, acct)
      if err != nil {
         return "", fmt.Errorf("Error fetching account (%s) media: %s", rssFeed.DbUsername, err)
      }

      // Insert the settings!
      if err := n.state.DB.PutAccountSettings(ctx, acct.Settings); err != nil {
         return "", err
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
         Email:                  rssFeed.DbUsername + "@rss.tooter.com",
         ConfirmedAt:            time.Now(),
         Approved:               &[]bool{true}[0],
      }

      // insert the user!
      return rssFeed.DbUsername, n.state.DB.PutUser(ctx, u)
   }

   return alreadyExistName, err
}
