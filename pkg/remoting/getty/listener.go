/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package getty

import (
	"context"
	"sync"

	getty "github.com/apache/dubbo-getty"
	"github.com/seata/seata-go/pkg/config"
	"github.com/seata/seata-go/pkg/protocol/codec"
	"github.com/seata/seata-go/pkg/protocol/message"
	"github.com/seata/seata-go/pkg/remoting/processor"
	"github.com/seata/seata-go/pkg/util/log"
	"go.uber.org/atomic"
)

var (
	clientHandler     *gettyClientHandler
	onceClientHandler = &sync.Once{}
)

type gettyClientHandler struct {
	conf           *config.ClientConfig
	idGenerator    *atomic.Uint32
	msgFutures     *sync.Map
	mergeMsgMap    *sync.Map
	sessionManager *SessionManager
	processorMap   map[message.MessageType]processor.RemotingProcessor
}

func GetGettyClientHandlerInstance() *gettyClientHandler {
	if clientHandler == nil {
		onceClientHandler.Do(func() {
			clientHandler = &gettyClientHandler{
				conf:           config.GetDefaultClientConfig("seata-go"),
				idGenerator:    &atomic.Uint32{},
				msgFutures:     &sync.Map{},
				mergeMsgMap:    &sync.Map{},
				sessionManager: sessionManager,
				processorMap:   make(map[message.MessageType]processor.RemotingProcessor, 0),
			}
		})
	}
	return clientHandler
}

func (g *gettyClientHandler) OnOpen(session getty.Session) error {
	log.Infof("Open new getty session ")
	g.sessionManager.registerSession(session)
	go func() {
		request := message.RegisterTMRequest{AbstractIdentifyRequest: message.AbstractIdentifyRequest{
			Version:                 g.conf.SeataVersion,
			ApplicationId:           g.conf.ApplicationID,
			TransactionServiceGroup: g.conf.TransactionServiceGroup,
		}}
		err := GetGettyRemotingClient().SendAsyncRequest(request)
		if err != nil {
			log.Errorf("OnOpen error: {%#v}", err.Error())
			g.sessionManager.releaseSession(session)
			return
		}
	}()

	return nil
}

func (g *gettyClientHandler) OnError(session getty.Session, err error) {
	log.Infof("session{%s} got error{%v}, will be closed.", session.Stat(), err)
	g.sessionManager.releaseSession(session)
}

func (g *gettyClientHandler) OnClose(session getty.Session) {
	log.Infof("session{%s} is closing......", session.Stat())
	g.sessionManager.releaseSession(session)
}

func (g *gettyClientHandler) OnMessage(session getty.Session, pkg interface{}) {
	ctx := context.Background()
	log.Debug("received message: {%#v}", pkg)

	rpcMessage, ok := pkg.(message.RpcMessage)
	if !ok {
		log.Errorf("received message is not protocol.RpcMessage. pkg: %#v", pkg)
		return
	}

	if mm, ok := rpcMessage.Body.(message.MessageTypeAware); ok {
		processor := g.processorMap[mm.GetTypeCode()]
		if processor != nil {
			processor.Process(ctx, rpcMessage)
		} else {
			log.Errorf("This message type %v has no processor.", mm.GetTypeCode())
		}
	} else {
		log.Errorf("This rpcMessage body %#v is not MessageTypeAware type.", rpcMessage.Body)
	}
}

func (g *gettyClientHandler) OnCron(session getty.Session) {
	log.Debug("session{%s} Oncron executing", session.Stat())
	g.transferBeatHeart(session, message.HeartBeatMessagePing)
}

func (g *gettyClientHandler) transferBeatHeart(session getty.Session, msg message.HeartBeatMessage) {
	rpcMessage := message.RpcMessage{
		ID:         int32(g.idGenerator.Inc()),
		Type:       message.GettyRequestTypeHeartbeatRequest,
		Codec:      byte(codec.CodecTypeSeata),
		Compressor: 0,
		Body:       msg,
	}
	GetGettyRemotingInstance().SendASync(rpcMessage, session, nil)
}

func (g *gettyClientHandler) RegisterProcessor(msgType message.MessageType, processor processor.RemotingProcessor) {
	if nil != processor {
		g.processorMap[msgType] = processor
	}
}
