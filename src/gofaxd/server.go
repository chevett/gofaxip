// This file is part of the GOfax.IP project - https://github.com/gonicus/gofaxip
// Copyright (C) 2014 GONICUS GmbH, Germany - http://www.gonicus.de
//
// This program is free software; you can redistribute it and/or
// modify it under the terms of the GNU General Public License
// as published by the Free Software Foundation; version 2
// of the License.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program; if not, write to the Free Software
// Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301, USA.

package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/fiorix/go-eventsocket/eventsocket"
	"gofaxlib"
	"gofaxlib/logger"
	// "os/exec"
	// "path/filepath"
	// "strconv"
)

const (
	recvqFileFormat   = "fax%08d.tif"
	recvqDir          = "recvq"
	defaultFaxrcvdCmd = "bin/faxrcvd"
	defaultDevice     = "freeswitch"
)

// EventSocketServer is a server for handling outgoing event socket connections from FreeSWITCH
type EventSocketServer struct {
	errorChan chan error
	killChan  chan struct{}
}

// NewEventSocketServer initializes a EventSocketServer
func NewEventSocketServer() *EventSocketServer {
	e := new(EventSocketServer)
	e.errorChan = make(chan error)
	e.killChan = make(chan struct{})
	return e
}

// Start starts a goroutine to listen for ESL connections and handle incoming calls
func (e *EventSocketServer) Start() {
	go func() {
		err := eventsocket.ListenAndServe(gofaxlib.Config.Gofaxd.Socket, e.handler)
		if err != nil {
			e.errorChan <- err
		}
	}()
}

// Errors returns a channel of fatal errors that make the server stop
func (e *EventSocketServer) Errors() <-chan error {
	return e.errorChan
}

// Kill aborts all running connections and kills the
// corresponding FreeSWITCH channels.
// TODO: Right now we have not way implemented to wait until
// all connections have closed and signal to the caller,
// so we have to wait a few seconds after calling Kill()
func (e *EventSocketServer) Kill() {
	close(e.killChan)
}

func (e *EventSocketServer) getExtension(c *eventsocket.Connection) string {
	audioFile := "/home/admin/code/gofaxip/big_bopper_hello_baby.wav"
	extension := "";

	c.Send("event plain DTMF")
	c.Execute("sleep", "1000", true);
	c.Execute("answer", "", true);

	ev, err := c.Execute("playback", audioFile, true)
	if err != nil {
		logger.Logger.Printf("playback error")
	}
	ev.PrettyPrint()

	es := gofaxlib.NewEventStream(c)

ExtensionEvents:
	for {
		select {
		case ev := <-es.Events():
			if ev.Get("Content-Type") == "text/disconnect-notice" {
				logger.Logger.Printf("Received disconnect message")
			} else {

				if ev.Get("Dtmf-Source") == "RTP" {
					// logger.Logger.Printf("------------------------")
					// logger.Logger.Printf(ev.Get("Event-Name"))
					logger.Logger.Printf("key press: " + ev.Get("Dtmf-Digit") + " " + ev.Get("Dtmf-Source"))
					// logger.Logger.Printf(ev.Get("Dtmf-Source"))
					//ev.PrettyPrint();
					// logger.Logger.Printf("========================")
					extension :=  extension + ev.Get("Dtmf-Digit")
					if len(extension) >= 3 {
						break ExtensionEvents
					}
				}
			}
		case err := <-es.Errors():
			if err.Error() == "EOF" {
				logger.Logger.Printf("Event socket client disconnected")
			} else {
				logger.Logger.Printf("Error:", err)
			}
			break ExtensionEvents
		case _ = <-e.killChan:
			logger.Logger.Printf("Kill reqeust received, destroying channel")
			c.Close()
			return "error" // sorry, i don't know what im doing here
		}
	}

	return extension
}

// Handle incoming call
func (e *EventSocketServer) handler(c *eventsocket.Connection) {
	logger.Logger.Println("Incoming Event Socket connection from", c.RemoteAddr())

	connectev, err := c.Send("connect") // Returns: Ganzer Event mit alles
	if err != nil {
		c.Send("exit")
		logger.Logger.Print(err)
		return
	}

	channelUUID := uuid.Parse(connectev.Get("Unique-Id"))
	if channelUUID == nil {
		c.Send("exit")
		logger.Logger.Print(err)
		return
	}
	defer logger.Logger.Println(channelUUID, "Handler ending")

	// Extract Caller/Callee
	recipient := connectev.Get("Variable_sip_to_user")
	cidname := connectev.Get("Channel-Caller-Id-Name")
	cidnum := connectev.Get("Channel-Caller-Id-Number")

	logger.Logger.Printf("Incoming call to %v from %v <%v>", recipient, cidname, cidnum)

	// Filter events
	c.Send("linger")
	c.Send(fmt.Sprintf("filter Unique-ID %v", channelUUID))

	extension := e.getExtension(c)
	logger.Logger.Printf("Got an extension: %v", extension)


	return
}
