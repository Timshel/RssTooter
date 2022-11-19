package rss

import (
   "context"
   "errors"
   "fmt"

   "github.com/superseriousbusiness/gotosocial/internal/config"
   "github.com/superseriousbusiness/gotosocial/internal/federation/dereferencing"
   "github.com/superseriousbusiness/gotosocial/internal/filter/visibility"
   "github.com/superseriousbusiness/gotosocial/internal/httpclient"
   "github.com/superseriousbusiness/gotosocial/internal/media"
   "github.com/superseriousbusiness/gotosocial/internal/state"
   "github.com/superseriousbusiness/gotosocial/internal/transport"
   "github.com/superseriousbusiness/gotosocial/internal/typeutils"
)

// generate RSA keys of this length
const rsaKeyBits = 2048

// Logic to proxy a Nitter instance
type RssTooter interface {

   // Start starts the RssTooter, start fetching Status from Nitter
   Start() error

   // Stop stops the RssTooter cleanly
   Stop() error

   NewUser(ctx context.Context, username string) (string, error)
}

// RssTooter just implements the RssTooter interface
type rssTooter struct {
   state                *state.State
   dereferencer         dereferencing.Dereferencer
   httpclient           *httpclient.Client
   ctx                  context.Context
   cancelFunc           context.CancelFunc
   transportController  transport.Controller

   nitterHost     string
   userPassword   string
   pollFrequency  int
}

// NewRssTooter returns a new RssTooter.
func NewRssTooter(
   pCtx                 context.Context,
   state                *state.State,
   mediaManager         *media.Manager,
   transportController  transport.Controller, 
   typeConverter        *typeutils.Converter,
   visFilter            *visibility.Filter,
) RssTooter {
   ctx, cancelFunc := context.WithCancel(pCtx)

   return &rssTooter{
      state:                  state,
      dereferencer:           dereferencing.NewDereferencer(state, typeConverter, transportController, visFilter, mediaManager),
      httpclient:             httpclient.New(httpclient.Config{ MaxOpenConnsPerHost: 1, }),
      ctx:                    ctx,
      cancelFunc:             cancelFunc,
      transportController:    transportController,
      userPassword:           config.GetRssUserPassword(),
      pollFrequency:          config.GetRssPollFrequency(),
   }
}

// Start starts the RssTooter, start fetching Status from Nitter
func (n *rssTooter) Start() error {
   if( n.userPassword == "" ){
      return errors.New(fmt.Sprintf("Missing %s config", config.RssUserPasswordFlag()))
   }

   if( n.pollFrequency == 0 ){
      return errors.New(fmt.Sprintf("Missing or invalid %s config %s", config.RssPollFrequencyFlag(), n.pollFrequency))
   }

   go n.refresh()
   return nil
}

// Stop stops the RssTooter cleanly
func (n *rssTooter) Stop() error {
   n.cancelFunc()
   return nil
}
