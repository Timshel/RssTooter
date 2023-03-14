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

package email

import (
	"bytes"
	"text/template"

	"github.com/superseriousbusiness/gotosocial/internal/config"
	"github.com/superseriousbusiness/gotosocial/internal/log"
)

// NewNoopSender returns a no-op email sender that will just execute the given sendCallback
// every time it would otherwise send an email to the given toAddress with the given message value.
//
// Passing a nil function is also acceptable, in which case the send functions will just return nil.
func NewNoopSender(sendCallback func(toAddress string, message string)) (Sender, error) {
	templateBaseDir := config.GetWebTemplateBaseDir()

	t, err := loadTemplates(templateBaseDir)
	if err != nil {
		return nil, err
	}

	return &noopSender{
		sendCallback: sendCallback,
		template:     t,
	}, nil
}

type noopSender struct {
	sendCallback func(toAddress string, message string)
	template     *template.Template
}

func (s *noopSender) SendConfirmEmail(toAddress string, data ConfirmData) error {
	buf := &bytes.Buffer{}
	if err := s.template.ExecuteTemplate(buf, confirmTemplate, data); err != nil {
		return err
	}
	confirmBody := buf.String()

	msg, err := assembleMessage(confirmSubject, confirmBody, toAddress, "test@example.org")
	if err != nil {
		return err
	}

	log.Tracef(nil, "NOT SENDING confirmation email to %s with contents: %s", toAddress, msg)

	if s.sendCallback != nil {
		s.sendCallback(toAddress, string(msg))
	}
	return nil
}

func (s *noopSender) SendResetEmail(toAddress string, data ResetData) error {
	buf := &bytes.Buffer{}
	if err := s.template.ExecuteTemplate(buf, resetTemplate, data); err != nil {
		return err
	}
	resetBody := buf.String()

	msg, err := assembleMessage(resetSubject, resetBody, toAddress, "test@example.org")
	if err != nil {
		return err
	}

	log.Tracef(nil, "NOT SENDING reset email to %s with contents: %s", toAddress, msg)

	if s.sendCallback != nil {
		s.sendCallback(toAddress, string(msg))
	}

	return nil
}

func (s *noopSender) SendTestEmail(toAddress string, data TestData) error {
	buf := &bytes.Buffer{}
	if err := s.template.ExecuteTemplate(buf, testTemplate, data); err != nil {
		return err
	}
	testBody := buf.String()

	msg, err := assembleMessage(testSubject, testBody, toAddress, "test@example.org")
	if err != nil {
		return err
	}

	log.Tracef(nil, "NOT SENDING test email to %s with contents: %s", toAddress, msg)

	if s.sendCallback != nil {
		s.sendCallback(toAddress, string(msg))
	}

	return nil
}
