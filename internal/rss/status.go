package rss

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeberg.org/gruf/go-kv"
	"github.com/antchfx/htmlquery"
	"github.com/superseriousbusiness/gotosocial/internal/ap"
	"github.com/superseriousbusiness/gotosocial/internal/gtserror"
	"github.com/superseriousbusiness/gotosocial/internal/gtsmodel"
	"github.com/superseriousbusiness/gotosocial/internal/id"
	"github.com/superseriousbusiness/gotosocial/internal/log"
	"github.com/superseriousbusiness/gotosocial/internal/messages"
	"github.com/superseriousbusiness/gotosocial/internal/uris"
	"github.com/superseriousbusiness/gotosocial/internal/util"
)

func createMediaAttachement(ctx context.Context, text string) []*gtsmodel.MediaAttachment {
	var attachments []*gtsmodel.MediaAttachment

	doc, err := htmlquery.Parse(strings.NewReader(text))
	if err == nil {
		for _, imgNode := range htmlquery.Find(doc, "//img") {
			alt := htmlquery.SelectAttr(imgNode, "alt")
			if len(alt) == 0 {
				alt = htmlquery.SelectAttr(imgNode, "title")
			}
			attachments = append(attachments, &gtsmodel.MediaAttachment{
				RemoteURL: htmlquery.SelectAttr(imgNode, "src"),
				Description: alt,
			})
		}
	}

	log.Infof(ctx, "Attachments: ", attachments)


	return attachments
}

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

	accountURIs := uris.GenerateURIsForAccount(toCreate.Account.Username)
	statusId := id.NewULID()

	var text = ""
	if len(toCreate.Item.Description) > 0 {
		text = toCreate.Item.Description
	} else {
		text = toCreate.Item.Content
	}

	attachments := createMediaAttachement(ctx, text)
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

