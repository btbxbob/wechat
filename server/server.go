package server

import (
	"encoding/xml"
	"errors"
	"github.com/chanxuehong/wechat/message/request"
	"io"
	"io/ioutil"
	"net/http"
	"sync"
)

// 对于微信服务器推送过来的消息或者事件, 公众号服务程序就相当于服务器.
// 被动回复和处理各种事件功能都封装在这个结构里; Server 并发安全.
//  NOTE: 必须调用 NewServer() 创建对象!
type Server struct {
	setting ServerSetting

	// 对于微信服务器推送过来的请求, 处理过程中有些中间状态比较大的变量, 所以可以缓存起来.
	//  NOTE: require go1.3+ , 如果你的环境不满足这个条件, 可以自己实现一个简单的 Pool,
	//        see github.com/chanxuehong/util/pool
	bufferUnitPool *sync.Pool
}

func NewServer(setting *ServerSetting) *Server {
	if setting == nil {
		panic("setting == nil")
	}

	var srv Server
	srv.setting.initialize(setting)
	srv.bufferUnitPool = &sync.Pool{
		New: newBufferUnit,
	}

	return &srv
}

// Server 实现 http.Handler 接口
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST": // 处理从微信服务器推送过来的消息 ===================================
		var err error
		var signature, timestamp, nonce string

		if err = r.ParseForm(); err != nil {
			s.setting.InvalidRequestHandler(w, r, err)
			return
		}

		if signature = r.FormValue("signature"); signature == "" {
			s.setting.InvalidRequestHandler(w, r, errors.New("signature is empty"))
			return
		}
		if timestamp = r.FormValue("timestamp"); timestamp == "" {
			s.setting.InvalidRequestHandler(w, r, errors.New("timestamp is empty"))
			return
		}
		if nonce = r.FormValue("nonce"); nonce == "" {
			s.setting.InvalidRequestHandler(w, r, errors.New("nonce is empty"))
			return
		}

		bufferUnit := s.getBufferUnitFromPool() // *serverBufferUnit
		defer s.putBufferUnitToPool(bufferUnit) // important!

		if !CheckSignatureEx(signature, timestamp, nonce, s.setting.Token, bufferUnit.buf) {
			s.setting.InvalidRequestHandler(w, r, errors.New("check signature failed"))
			return
		}

		requestBody, err := ioutil.ReadAll(r.Body)
		if err != nil {
			s.setting.InvalidRequestHandler(w, r, err)
			return
		}

		msgRqst := &bufferUnit.msgRequest
		if err = xml.Unmarshal(requestBody, msgRqst); err != nil {
			s.setting.InvalidRequestHandler(w, r, err)
			return
		}

		// request router, 可一个根据自己的实际业务调整顺序!
		switch msgRqst.MsgType {
		case request.MSG_TYPE_TEXT:
			s.setting.TextRequestHandler(w, r, msgRqst.Text())

		case request.MSG_TYPE_EVENT:
			// event router
			switch msgRqst.Event {
			case request.EVENT_TYPE_CLICK:
				s.setting.MenuClickEventHandler(w, r, msgRqst.MenuClickEvent())

			case request.EVENT_TYPE_VIEW:
				s.setting.MenuViewEventHandler(w, r, msgRqst.MenuViewEvent())

			case request.EVENT_TYPE_LOCATION:
				s.setting.LocationEventHandler(w, r, msgRqst.LocationEvent())

			case request.EVENT_TYPE_MERCHANTORDER:
				s.setting.MerchantOrderEventHandler(w, r, msgRqst.MerchantOrderEvent())

			case request.EVENT_TYPE_SUBSCRIBE:
				if msgRqst.Ticket == "" { // 普通订阅
					s.setting.SubscribeEventHandler(w, r, msgRqst.SubscribeEvent())
				} else { // 扫描二维码订阅
					s.setting.SubscribeByScanEventHandler(w, r, msgRqst.SubscribeByScanEvent())
				}

			case request.EVENT_TYPE_UNSUBSCRIBE:
				s.setting.UnsubscribeEventHandler(w, r, msgRqst.UnsubscribeEvent())

			case request.EVENT_TYPE_SCAN:
				s.setting.ScanEventHandler(w, r, msgRqst.ScanEvent())

			case request.EVENT_TYPE_MASSSENDJOBFINISH:
				s.setting.MassSendJobFinishEventHandler(w, r, msgRqst.MassSendJobFinishEvent())

			default: // unknown event
				s.setting.UnknownRequestHandler(w, r, requestBody)
			}

		case request.MSG_TYPE_LINK:
			s.setting.LinkRequestHandler(w, r, msgRqst.Link())

		case request.MSG_TYPE_VOICE:
			if msgRqst.Recognition == "" { // 普通的语音请求
				s.setting.VoiceRequestHandler(w, r, msgRqst.Voice())
			} else { // 语音识别请求
				s.setting.VoiceRecognitionRequestHandler(w, r, msgRqst.VoiceRecognition())
			}

		case request.MSG_TYPE_LOCATION:
			s.setting.LocationRequestHandler(w, r, msgRqst.Location())

		case request.MSG_TYPE_IMAGE:
			s.setting.ImageRequestHandler(w, r, msgRqst.Image())

		case request.MSG_TYPE_VIDEO:
			s.setting.VideoRequestHandler(w, r, msgRqst.Video())

		default: // unknown request message type
			s.setting.UnknownRequestHandler(w, r, requestBody)
		}

	case "GET": // 首次验证 ======================================================
		var err error
		var signature, timestamp, nonce, echostr string

		if err = r.ParseForm(); err != nil {
			s.setting.InvalidRequestHandler(w, r, err)
			return
		}

		if signature = r.FormValue("signature"); signature == "" {
			s.setting.InvalidRequestHandler(w, r, errors.New("signature is empty"))
			return
		}
		if timestamp = r.FormValue("timestamp"); timestamp == "" {
			s.setting.InvalidRequestHandler(w, r, errors.New("timestamp is empty"))
			return
		}
		if nonce = r.FormValue("nonce"); nonce == "" {
			s.setting.InvalidRequestHandler(w, r, errors.New("nonce is empty"))
			return
		}
		if echostr = r.FormValue("echostr"); echostr == "" {
			s.setting.InvalidRequestHandler(w, r, errors.New("echostr is empty"))
			return
		}

		bufferUnit := s.getBufferUnitFromPool() // *serverBufferUnit
		defer s.putBufferUnitToPool(bufferUnit) // important!

		if !CheckSignatureEx(signature, timestamp, nonce, s.setting.Token, bufferUnit.buf) {
			s.setting.InvalidRequestHandler(w, r, errors.New("check signature failed"))
			return
		}

		io.WriteString(w, echostr)
	}
}
