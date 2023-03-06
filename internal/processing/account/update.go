/*
   GoToSocial
   Copyright (C) 2021-2023 GoToSocial Authors admin@gotosocial.org

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package account

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"

	"github.com/superseriousbusiness/gotosocial/internal/ap"
	apimodel "github.com/superseriousbusiness/gotosocial/internal/api/model"
	"github.com/superseriousbusiness/gotosocial/internal/config"
	"github.com/superseriousbusiness/gotosocial/internal/gtserror"
	"github.com/superseriousbusiness/gotosocial/internal/gtsmodel"
	"github.com/superseriousbusiness/gotosocial/internal/log"
	"github.com/superseriousbusiness/gotosocial/internal/media"
	"github.com/superseriousbusiness/gotosocial/internal/messages"
	"github.com/superseriousbusiness/gotosocial/internal/text"
	"github.com/superseriousbusiness/gotosocial/internal/typeutils"
	"github.com/superseriousbusiness/gotosocial/internal/validate"
)

// Update processes the update of an account with the given form.
func (p *Processor) Update(ctx context.Context, account *gtsmodel.Account, form *apimodel.UpdateCredentialsRequest) (*apimodel.Account, gtserror.WithCode) {
	if form.Discoverable != nil {
		account.Discoverable = form.Discoverable
	}

	if form.Bot != nil {
		account.Bot = form.Bot
	}

	reparseEmojis := false

	if form.DisplayName != nil {
		if err := validate.DisplayName(*form.DisplayName); err != nil {
			return nil, gtserror.NewErrorBadRequest(err)
		}
		account.DisplayName = text.SanitizePlaintext(*form.DisplayName)
		reparseEmojis = true
	}

	if form.Note != nil {
		if err := validate.Note(*form.Note); err != nil {
			return nil, gtserror.NewErrorBadRequest(err)
		}

		// Set the raw note before processing
		account.NoteRaw = *form.Note
		reparseEmojis = true
	}

	if reparseEmojis {
		// If either DisplayName or Note changed, reparse both, because we
		// can't otherwise tell which one each emoji belongs to.
		// Deduplicate emojis between the two fields.
		emojis := make(map[string]*gtsmodel.Emoji)
		formatResult := p.formatter.FromPlainEmojiOnly(ctx, p.parseMention, account.ID, "", account.DisplayName)
		for _, emoji := range formatResult.Emojis {
			emojis[emoji.ID] = emoji
		}

		// Process note to generate a valid HTML representation
		var f text.FormatFunc
		if account.StatusContentType == "text/markdown" {
			f = p.formatter.FromMarkdown
		} else {
			f = p.formatter.FromPlain
		}
		formatted := f(ctx, p.parseMention, account.ID, "", account.NoteRaw)

		// Set updated HTML-ified note
		account.Note = formatted.HTML
		for _, emoji := range formatted.Emojis {
			emojis[emoji.ID] = emoji
		}

		account.Emojis = []*gtsmodel.Emoji{}
		account.EmojiIDs = []string{}
		for eid, emoji := range emojis {
			account.Emojis = append(account.Emojis, emoji)
			account.EmojiIDs = append(account.EmojiIDs, eid)
		}
	}

	if form.Avatar != nil && form.Avatar.Size != 0 {
		avatarInfo, err := p.UpdateAvatar(ctx, form.Avatar, nil, account.ID)
		if err != nil {
			return nil, gtserror.NewErrorBadRequest(err)
		}
		account.AvatarMediaAttachmentID = avatarInfo.ID
		account.AvatarMediaAttachment = avatarInfo
		log.Tracef(ctx, "new avatar info for account %s is %+v", account.ID, avatarInfo)
	}

	if form.Header != nil && form.Header.Size != 0 {
		headerInfo, err := p.UpdateHeader(ctx, form.Header, nil, account.ID)
		if err != nil {
			return nil, gtserror.NewErrorBadRequest(err)
		}
		account.HeaderMediaAttachmentID = headerInfo.ID
		account.HeaderMediaAttachment = headerInfo
		log.Tracef(ctx, "new header info for account %s is %+v", account.ID, headerInfo)
	}

	if form.Locked != nil {
		account.Locked = form.Locked
	}

	if form.Source != nil {
		if form.Source.Language != nil {
			if err := validate.Language(*form.Source.Language); err != nil {
				return nil, gtserror.NewErrorBadRequest(err)
			}
			account.Language = *form.Source.Language
		}

		if form.Source.Sensitive != nil {
			account.Sensitive = form.Source.Sensitive
		}

		if form.Source.Privacy != nil {
			if err := validate.Privacy(*form.Source.Privacy); err != nil {
				return nil, gtserror.NewErrorBadRequest(err)
			}
			privacy := typeutils.APIVisToVis(apimodel.Visibility(*form.Source.Privacy))
			account.Privacy = privacy
		}

		if form.Source.StatusContentType != nil {
			if err := validate.StatusContentType(*form.Source.StatusContentType); err != nil {
				return nil, gtserror.NewErrorBadRequest(err, err.Error())
			}

			account.StatusContentType = *form.Source.StatusContentType
		}
	}

	if form.CustomCSS != nil {
		customCSS := *form.CustomCSS
		if err := validate.CustomCSS(customCSS); err != nil {
			return nil, gtserror.NewErrorBadRequest(err, err.Error())
		}
		account.CustomCSS = text.SanitizePlaintext(customCSS)
	}

	if form.EnableRSS != nil {
		account.EnableRSS = form.EnableRSS
	}

	if form.FieldsAttributes != nil && len(*form.FieldsAttributes) != 0 {
		if err := validate.ProfileFieldsCount(*form.FieldsAttributes); err != nil {
			return nil, gtserror.NewErrorBadRequest(err)
		}

		account.Fields = make([]gtsmodel.Field, 0) // reset fields
		for _, f := range *form.FieldsAttributes {
			if f.Name != nil && f.Value != nil {
				if *f.Name != "" && *f.Value != "" {
					field := gtsmodel.Field{}

					field.Name = validate.ProfileField(f.Name)
					field.Value = validate.ProfileField(f.Value)

					account.Fields = append(account.Fields, field)
				}
			}
		}
	}

	err := p.state.DB.UpdateAccount(ctx, account)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(fmt.Errorf("could not update account %s: %s", account.ID, err))
	}

	p.state.Workers.EnqueueClientAPI(ctx, messages.FromClientAPI{
		APObjectType:   ap.ObjectProfile,
		APActivityType: ap.ActivityUpdate,
		GTSModel:       account,
		OriginAccount:  account,
	})

	acctSensitive, err := p.tc.AccountToAPIAccountSensitive(ctx, account)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(fmt.Errorf("could not convert account into apisensitive account: %s", err))
	}
	return acctSensitive, nil
}

// UpdateAvatar does the dirty work of checking the avatar part of an account update form,
// parsing and checking the image, and doing the necessary updates in the database for this to become
// the account's new avatar image.
func (p *Processor) UpdateAvatar(ctx context.Context, avatar *multipart.FileHeader, description *string, accountID string) (*gtsmodel.MediaAttachment, error) {
	maxImageSize := config.GetMediaImageMaxSize()
	if avatar.Size > int64(maxImageSize) {
		return nil, fmt.Errorf("UpdateAvatar: avatar with size %d exceeded max image size of %d bytes", avatar.Size, maxImageSize)
	}

	dataFunc := func(innerCtx context.Context) (io.ReadCloser, int64, error) {
		f, err := avatar.Open()
		return f, avatar.Size, err
	}

	isAvatar := true
	ai := &media.AdditionalMediaInfo{
		Avatar:      &isAvatar,
		Description: description,
	}

	processingMedia, err := p.mediaManager.PreProcessMedia(ctx, dataFunc, nil, accountID, ai)
	if err != nil {
		return nil, fmt.Errorf("UpdateAvatar: error processing avatar: %s", err)
	}

	return processingMedia.LoadAttachment(ctx)
}

// UpdateHeader does the dirty work of checking the header part of an account update form,
// parsing and checking the image, and doing the necessary updates in the database for this to become
// the account's new header image.
func (p *Processor) UpdateHeader(ctx context.Context, header *multipart.FileHeader, description *string, accountID string) (*gtsmodel.MediaAttachment, error) {
	maxImageSize := config.GetMediaImageMaxSize()
	if header.Size > int64(maxImageSize) {
		return nil, fmt.Errorf("UpdateHeader: header with size %d exceeded max image size of %d bytes", header.Size, maxImageSize)
	}

	dataFunc := func(innerCtx context.Context) (io.ReadCloser, int64, error) {
		f, err := header.Open()
		return f, header.Size, err
	}

	isHeader := true
	ai := &media.AdditionalMediaInfo{
		Header: &isHeader,
	}

	processingMedia, err := p.mediaManager.PreProcessMedia(ctx, dataFunc, nil, accountID, ai)
	if err != nil {
		return nil, fmt.Errorf("UpdateHeader: error processing header: %s", err)
	}

	return processingMedia.LoadAttachment(ctx)
}
