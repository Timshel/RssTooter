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

package bundb_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/superseriousbusiness/gotosocial/internal/config"
	"github.com/superseriousbusiness/gotosocial/internal/db"
)

type InstanceTestSuite struct {
	BunDBStandardTestSuite
}

func (suite *InstanceTestSuite) TestCountInstanceUsers() {
	count, err := suite.db.CountInstanceUsers(context.Background(), config.GetHost())
	suite.NoError(err)
	suite.Equal(4, count)
}

func (suite *InstanceTestSuite) TestCountInstanceUsersRemote() {
	count, err := suite.db.CountInstanceUsers(context.Background(), "fossbros-anonymous.io")
	suite.NoError(err)
	suite.Equal(1, count)
}

func (suite *InstanceTestSuite) TestCountInstanceStatuses() {
	count, err := suite.db.CountInstanceStatuses(context.Background(), config.GetHost())
	suite.NoError(err)
	suite.Equal(16, count)
}

func (suite *InstanceTestSuite) TestCountInstanceStatusesRemote() {
	count, err := suite.db.CountInstanceStatuses(context.Background(), "fossbros-anonymous.io")
	suite.NoError(err)
	suite.Equal(1, count)
}

func (suite *InstanceTestSuite) TestCountInstanceDomains() {
	count, err := suite.db.CountInstanceDomains(context.Background(), config.GetHost())
	suite.NoError(err)
	suite.Equal(2, count)
}

func (suite *InstanceTestSuite) TestGetInstanceOK() {
	instance, err := suite.db.GetInstance(context.Background(), "localhost:8080")
	suite.NoError(err)
	suite.NotNil(instance)
}

func (suite *InstanceTestSuite) TestGetInstanceNonexistent() {
	instance, err := suite.db.GetInstance(context.Background(), "doesnt.exist.com")
	suite.ErrorIs(err, db.ErrNoEntries)
	suite.Nil(instance)
}

func (suite *InstanceTestSuite) TestGetInstancePeers() {
	peers, err := suite.db.GetInstancePeers(context.Background(), false)
	suite.NoError(err)
	suite.Len(peers, 2)
}

func (suite *InstanceTestSuite) TestGetInstancePeersIncludeSuspended() {
	peers, err := suite.db.GetInstancePeers(context.Background(), true)
	suite.NoError(err)
	suite.Len(peers, 2)
}

func (suite *InstanceTestSuite) TestGetInstanceAccounts() {
	accounts, err := suite.db.GetInstanceAccounts(context.Background(), "fossbros-anonymous.io", "", 10)
	suite.NoError(err)
	suite.Len(accounts, 1)
}

func TestInstanceTestSuite(t *testing.T) {
	suite.Run(t, new(InstanceTestSuite))
}
