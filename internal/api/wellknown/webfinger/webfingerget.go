// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package webfinger

import (
	"errors"
	"fmt"
	"net/http"

	"codeberg.org/gruf/go-kv"
	"github.com/gin-gonic/gin"
	apiutil "github.com/superseriousbusiness/gotosocial/internal/api/util"
	"github.com/superseriousbusiness/gotosocial/internal/gtserror"
	"github.com/superseriousbusiness/gotosocial/internal/log"
)

// WebfingerGETRequest swagger:operation GET /.well-known/webfinger webfingerGet
//
// Handles webfinger account lookup requests.
//
// For example, a GET to `https://goblin.technology/.well-known/webfinger?resource=acct:tobi@goblin.technology` would return:
//
// ```
//
//	{"subject":"acct:tobi@goblin.technology","aliases":["https://goblin.technology/users/tobi","https://goblin.technology/@tobi"],"links":[{"rel":"http://webfinger.net/rel/profile-page","type":"text/html","href":"https://goblin.technology/@tobi"},{"rel":"self","type":"application/activity+json","href":"https://goblin.technology/users/tobi"}]}
//
// ```
//
// See: https://webfinger.net/
//
//	---
//	tags:
//	- .well-known
//
//	produces:
//	- application/jrd+json
//
//	responses:
//		'200':
//			schema:
//				"$ref": "#/definitions/wellKnownResponse"
func (m *Module) WebfingerGETRequest(c *gin.Context) {
	l := log.WithFields(kv.Fields{
		{K: "user-agent", V: c.Request.UserAgent()},
	}...)

	if _, err := apiutil.NegotiateAccept(c, apiutil.WebfingerJSONAcceptHeaders...); err != nil {
		apiutil.ErrorHandler(c, gtserror.NewErrorNotAcceptable(err, err.Error()), m.processor.InstanceGetV1)
		return
	}

	resourceQuery, set := c.GetQuery("resource")
	if !set || resourceQuery == "" {
		err := errors.New("no 'resource' in request query")
		apiutil.ErrorHandler(c, gtserror.NewErrorBadRequest(err, err.Error()), m.processor.InstanceGetV1)
		return
	}

	// RssTooter handling
	username, err := m.rssTooter.NewUser(c, resourceQuery)
	if err != nil {
		l.Errorf("Failed to create user for %s: %s", resourceQuery, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Failed to create user for %s", resourceQuery)})
		return
	}

	resp, errWithCode := m.processor.Fedi().WebfingerGet(c.Request.Context(), username)
	if errWithCode != nil {
		apiutil.ErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return
	}

	// Encode JSON HTTP response.
	apiutil.EncodeJSONResponse(
		c.Writer,
		c.Request,
		http.StatusOK,
		apiutil.AppJRDJSON,
		resp,
	)
}
