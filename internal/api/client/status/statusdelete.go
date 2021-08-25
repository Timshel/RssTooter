/*
   GoToSocial
   Copyright (C) 2021 GoToSocial Authors admin@gotosocial.org

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

package status

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/superseriousbusiness/gotosocial/internal/oauth"
)

// StatusDELETEHandler swagger:operation DELETE /api/v1/statuses/{id} statusDelete
//
// Delete status with the given ID. The status must belong to you.
//
// The deleted status will be returned in the response. The `text` field will contain the original text of the status as it was submitted.
// This is useful when doing a 'delete and redraft' type operation.
//
// ---
// tags:
// - statuses
//
// produces:
// - application/json
//
// parameters:
// - name: id
//   type: string
//   description: Target status ID.
//   in: path
//   required: true
//
// security:
// - OAuth2 Bearer:
//   - write:statuses
//
// responses:
//   '200':
//     description: "The newly deleted status."
//     schema:
//       "$ref": "#/definitions/status"
//   '400':
//      description: bad request
//   '401':
//      description: unauthorized
//   '403':
//      description: forbidden
//   '404':
//      description: not found
func (m *Module) StatusDELETEHandler(c *gin.Context) {
	l := m.log.WithFields(logrus.Fields{
		"func":        "StatusDELETEHandler",
		"request_uri": c.Request.RequestURI,
		"user_agent":  c.Request.UserAgent(),
		"origin_ip":   c.ClientIP(),
	})
	l.Debugf("entering function")

	authed, err := oauth.Authed(c, true, false, true, true) // we don't really need an app here but we want everything else
	if err != nil {
		l.Debug("not authed so can't delete status")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	targetStatusID := c.Param(IDKey)
	if targetStatusID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no status id provided"})
		return
	}

	mastoStatus, err := m.processor.StatusDelete(c.Request.Context(), authed, targetStatusID)
	if err != nil {
		l.Debugf("error processing status delete: %s", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
		return
	}

	// the status was already gone/never existed
	if mastoStatus == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Record not found"})
		return
	}

	c.JSON(http.StatusOK, mastoStatus)
}
