package rss

import (
	"context"
	"fmt"
	"time"

	"codeberg.org/gruf/go-kv"
	"github.com/superseriousbusiness/gotosocial/internal/ap"
	"github.com/superseriousbusiness/gotosocial/internal/gtserror"
	"github.com/superseriousbusiness/gotosocial/internal/gtsmodel"
	"github.com/superseriousbusiness/gotosocial/internal/id"
	"github.com/superseriousbusiness/gotosocial/internal/log"
	"github.com/superseriousbusiness/gotosocial/internal/messages"
	"github.com/superseriousbusiness/gotosocial/internal/uris"
	"github.com/superseriousbusiness/gotosocial/internal/util"
)

func (n *rssTooter) PutStatus(ctx context.Context, toCreate *ToCreate) error {
	l := log.WithFields(kv.Fields{
		{ K: "ID", V: toCreate.Account.ID,},
		{ K: "item", V: toCreate.Item.Link,},
	}...)

	// Pre-fetch a transport for requesting username, used by later dereferencing.
	tsport, err := n.transportController.NewTransportForUsername(ctx, toCreate.Account.Username)
	if err != nil {
		return gtserror.Newf("couldn't create transport: %w", err)
	}

	var attachments []*gtsmodel.MediaAttachment
	if toCreate.Item.Image != nil {
		l.Infof("Image URL: %s", toCreate.Item.Image.URL)
		attachments = append(attachments, &gtsmodel.MediaAttachment{ RemoteURL: toCreate.Item.Image.URL, })
	}

	accountURIs := uris.GenerateURIsForAccount(toCreate.Account.Username)
	statusId := id.NewULID()

	var text = ""
	if len(toCreate.Item.Description) > 0 {
		text = toCreate.Item.Description
	} else {
		text = toCreate.Item.Content
	}
	content := fmt.Sprintf(`<p><a href="%s">%s</a></p><p>%s</p>`, toCreate.Item.Link, toCreate.Item.Title, text)

	newStatus := &gtsmodel.Status{
		ID:                       statusId,
		URI:                      accountURIs.StatusesURI + "/" + statusId,
		URL:                      toCreate.Item.Link,
		Local:                    util.Ptr(true),
		Attachments:              attachments,
		CreatedAt:                *toCreate.Item.PublishedParsed,
		UpdatedAt:                time.Now(),
		Account:                  toCreate.Account,
		AccountID:                toCreate.Account.ID,
		AccountURI:               toCreate.Account.URI,
		ActivityStreamsType:      ap.ObjectNote,
		Content:  				  content,
		Text:                     toCreate.Item.Description,
		Visibility: 			  gtsmodel.VisibilityPublic,
		Sensitive:                &[]bool{false}[0],
		Federated: 				  &[]bool{true}[0],
		Boostable: 				  &[]bool{true}[0],
		Replyable: 				  &[]bool{false}[0],
		Likeable: 				  &[]bool{true}[0],
	}

	if errWithCode := n.processThreadID(ctx, newStatus); errWithCode != nil {
		return errWithCode
	}

	n.dereferencer.FetchStatusAttachments(n.ctx, tsport, newStatus, newStatus)

	// put the new status in the database
	l.Infof(fmt.Sprintf("Pushing item to DB (time: %s)", toCreate.Item.PublishedParsed))
	if err := n.state.DB.PutStatus(ctx, newStatus); err != nil {
		l.Errorf("Failed to push item to DB: %s", err)
		return gtserror.NewErrorInternalError(err)
	}

	// send it back to the client API worker for async side-effects.
	n.state.Workers.Client.Queue.Push(&messages.FromClientAPI{
		APObjectType:   ap.ObjectNote,
		APActivityType: ap.ActivityCreate,
		GTSModel:       newStatus,
		Origin:         toCreate.Account,
	})

	return nil
}


func (p *rssTooter) processThreadID(ctx context.Context, status *gtsmodel.Status) gtserror.WithCode {
	// Mark new thread (or threaded subsection) starting from here.
	threadID := id.NewULID()
	if err := p.state.DB.PutThread(
		ctx,
		&gtsmodel.Thread{
			ID: threadID,
		},
	); err != nil {
		err := gtserror.Newf("error inserting new thread in db: %w", err)
		return gtserror.NewErrorInternalError(err)
	}

	// Future replies to this status
	// (if any) will inherit this thread ID.
	status.ThreadID = threadID

	return nil
}

