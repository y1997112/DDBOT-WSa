package client

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/pkg/errors"

	"github.com/RomiChan/syncx"
	"github.com/gorilla/websocket"

	"github.com/Mrs4s/MiraiGo/binary"
	"github.com/Mrs4s/MiraiGo/client/internal/auth"
	"github.com/Mrs4s/MiraiGo/client/internal/highway"
	"github.com/Mrs4s/MiraiGo/client/internal/intern"
	"github.com/Mrs4s/MiraiGo/client/internal/network"
	"github.com/Mrs4s/MiraiGo/client/internal/oicq"
	"github.com/Mrs4s/MiraiGo/client/pb/msg"
	"github.com/Mrs4s/MiraiGo/message"
	"github.com/Mrs4s/MiraiGo/utils"
	"github.com/Sora233/MiraiGo-Template/config"
)

type QQClient struct {
	Uin         int64
	PasswordMd5 [16]byte

	stat Statistics
	once sync.Once

	// option
	AllowSlider        bool
	UseFragmentMessage bool

	// account info
	Online        atomic.Bool
	Nickname      string
	Age           uint16
	Gender        uint16
	FriendList    []*FriendInfo
	GroupList     []*GroupInfo
	OnlineClients []*OtherClientInfo
	QiDian        *QiDianAccountInfo
	GuildService  *GuildService

	// protocol public field
	SequenceId  atomic.Int32
	SessionId   []byte
	TCP         *network.TCPClient // todo: combine other protocol state into one struct
	ConnectTime time.Time

	transport *network.Transport
	oicq      *oicq.Codec
	logger    Logger

	// internal state
	handlers        syncx.Map[uint16, *handlerInfo]
	waiters         syncx.Map[string, func(any, error)]
	initServerOnce  sync.Once
	servers         []netip.AddrPort
	currServerIndex int
	retryTimes      int
	alive           bool

	// session info
	qwebSeq        atomic.Int64
	sig            *auth.SigInfo
	highwaySession *highway.Session
	// pwdFlag        bool
	// timeDiff       int64

	// address
	// otherSrvAddrs   []string
	// fileStorageInfo *jce.FileStoragePushFSSvcList

	// event handles
	eventHandlers                     eventHandlers
	PrivateMessageEvent               EventHandle[*message.PrivateMessage]
	TempMessageEvent                  EventHandle[*TempMessageEvent]
	GroupMessageEvent                 EventHandle[*message.GroupMessage]
	SelfPrivateMessageEvent           EventHandle[*message.PrivateMessage]
	SelfGroupMessageEvent             EventHandle[*message.GroupMessage]
	GroupMuteEvent                    EventHandle[*GroupMuteEvent]
	GroupMessageRecalledEvent         EventHandle[*GroupMessageRecalledEvent]
	FriendMessageRecalledEvent        EventHandle[*FriendMessageRecalledEvent]
	GroupJoinEvent                    EventHandle[*GroupInfo]
	GroupLeaveEvent                   EventHandle[*GroupLeaveEvent]
	GroupMemberJoinEvent              EventHandle[*MemberJoinGroupEvent]
	GroupMemberLeaveEvent             EventHandle[*MemberLeaveGroupEvent]
	MemberCardUpdatedEvent            EventHandle[*MemberCardUpdatedEvent]
	GroupNameUpdatedEvent             EventHandle[*GroupNameUpdatedEvent]
	GroupMemberPermissionChangedEvent EventHandle[*MemberPermissionChangedEvent]
	GroupInvitedEvent                 EventHandle[*GroupInvitedRequest]
	UserWantJoinGroupEvent            EventHandle[*UserJoinGroupRequest]
	NewFriendEvent                    EventHandle[*NewFriendEvent]
	NewFriendRequestEvent             EventHandle[*NewFriendRequest]
	DisconnectedEvent                 EventHandle[*ClientDisconnectedEvent]
	GroupNotifyEvent                  EventHandle[INotifyEvent]
	FriendNotifyEvent                 EventHandle[INotifyEvent]
	MemberSpecialTitleUpdatedEvent    EventHandle[*MemberSpecialTitleUpdatedEvent]
	GroupDigestEvent                  EventHandle[*GroupDigestEvent]
	OtherClientStatusChangedEvent     EventHandle[*OtherClientStatusChangedEvent]
	OfflineFileEvent                  EventHandle[*OfflineFileEvent]
	GroupDisbandEvent                 EventHandle[*GroupDisbandEvent]
	DeleteFriendEvent                 EventHandle[*DeleteFriendEvent]

	// message state
	msgSvcCache            *utils.Cache[unit]
	lastC2CMsgTime         int64
	transCache             *utils.Cache[unit]
	groupSysMsgCache       *GroupSystemMessages
	msgBuilders            syncx.Map[int32, *messageBuilder]
	onlinePushCache        *utils.Cache[unit]
	heartbeatEnabled       bool
	requestPacketRequestID atomic.Int32
	groupSeq               atomic.Int32
	friendSeq              atomic.Int32
	highwayApplyUpSeq      atomic.Int32

	groupListLock sync.Mutex
	//增加的字段
	ws              *websocket.Conn
	responseChans   map[string]chan *ResponseGroupData
	responseMembers map[string]chan *ResponseGroupMemberData
	currentEcho     string
}

// 新增
type GroupData struct {
	GroupCreateTime int64  `json:"group_create_time"`
	GroupID         int64  `json:"group_id"`
	GroupLevel      int    `json:"group_level"`
	GroupName       string `json:"group_name"`
	MaxMemberCount  int    `json:"max_member_count"`
	MemberCount     int    `json:"member_count"`
	// ... and other fields ...
}

// 新增
type ResponseGroupData struct {
	Data    []GroupData `json:"data"`
	Message string      `json:"message"`
	Retcode int         `json:"retcode"`
	Status  string      `json:"status"`
	Echo    string      `json:"echo"`
}

// 群成员信息
type MemberData struct {
	GroupID         int64  `json:"group_id"`
	UserID          int64  `json:"user_id"`
	Nickname        string `json:"nickname"`
	Card            string `json:"card"`
	Sex             string `json:"sex"`
	Age             int    `json:"age"`
	Area            string `json:"area"`
	Level           int16  `json:"level"`
	QQLevel         int16  `json:"qq_level"`
	JoinTime        int64  `json:"join_time"`
	LastSentTime    int64  `json:"last_sent_time"`
	TitleExpireTime int64  `json:"title_expire_time"`
	Unfriendly      bool   `json:"unfriendly"`
	CardChangeable  bool   `json:"card_changeable"`
	IsRobot         bool   `json:"is_robot"`
	ShutUpTimestamp int64  `json:"shut_up_timestamp"`
	Role            string `json:"role"`
	Title           string `json:"title"`
}

// 群成员请求返回
type ResponseGroupMemberData struct {
	Status  string       `json:"status"`
	Retcode int          `json:"retcode"`
	Message string       `json:"message"`
	Wording string       `json:"wording"`
	Data    []MemberData `json:"data"`
}

// 新增
type BasicMessage struct {
	Echo string          `json:"echo"`
	Data json.RawMessage `json:"data"`
}

type QiDianAccountInfo struct {
	MasterUin  int64
	ExtName    string
	CreateTime int64

	bigDataReqAddrs   []string
	bigDataReqSession *bigDataSessionInfo
}

type handlerInfo struct {
	fun     func(i any, err error)
	dynamic bool
	params  network.RequestParams
}

func (h *handlerInfo) getParams() network.RequestParams {
	if h == nil {
		return nil
	}
	return h.params
}

var decoders = map[string]func(*QQClient, *network.Packet) (any, error){
	"wtlogin.login":                                decodeLoginResponse,
	"wtlogin.exchange_emp":                         decodeExchangeEmpResponse,
	"wtlogin.trans_emp":                            decodeTransEmpResponse,
	"StatSvc.register":                             decodeClientRegisterResponse,
	"StatSvc.ReqMSFOffline":                        decodeMSFOfflinePacket,
	"MessageSvc.PushNotify":                        decodeSvcNotify,
	"OnlinePush.ReqPush":                           decodeOnlinePushReqPacket,
	"OnlinePush.PbPushTransMsg":                    decodeOnlinePushTransPacket,
	"OnlinePush.SidTicketExpired":                  decodeSidExpiredPacket,
	"ConfigPushSvc.PushReq":                        decodePushReqPacket,
	"MessageSvc.PbGetMsg":                          decodeMessageSvcPacket,
	"MessageSvc.PushForceOffline":                  decodeForceOfflinePacket,
	"PbMessageSvc.PbMsgWithDraw":                   decodeMsgWithDrawResponse,
	"friendlist.getFriendGroupList":                decodeFriendGroupListResponse,
	"friendlist.delFriend":                         decodeFriendDeleteResponse,
	"friendlist.GetTroopListReqV2":                 decodeGroupListResponse,
	"friendlist.GetTroopMemberListReq":             decodeGroupMemberListResponse,
	"group_member_card.get_group_member_card_info": decodeGroupMemberInfoResponse,
	"LongConn.OffPicUp":                            decodeOffPicUpResponse,
	"ProfileService.Pb.ReqSystemMsgNew.Group":      decodeSystemMsgGroupPacket,
	"ProfileService.Pb.ReqSystemMsgNew.Friend":     decodeSystemMsgFriendPacket,
	"OidbSvc.0xd79":                                decodeWordSegmentation,
	"OidbSvc.0x990":                                decodeTranslateResponse,
	"SummaryCard.ReqSummaryCard":                   decodeSummaryCardResponse,
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var md5Int64Mapping = make(map[int64]string)
var md5Int64MappingLock sync.Mutex

type DynamicInt64 int64

type MessageContent struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

type WebSocketMessage struct {
	PostType    string       `json:"post_type"`
	MessageType string       `json:"message_type"`
	Time        DynamicInt64 `json:"time"`
	SelfID      DynamicInt64 `json:"self_id"`
	SubType     string       `json:"sub_type"`
	Sender      struct {
		Age      int          `json:"age"`
		Card     string       `json:"card"`
		Nickname string       `json:"nickname"`
		Role     string       `json:"role"`
		UserID   DynamicInt64 `json:"user_id"`
	} `json:"sender"`
	UserID         DynamicInt64 `json:"user_id"`
	MessageID      DynamicInt64 `json:"message_id"`
	GroupID        DynamicInt64 `json:"group_id"`
	MessageContent interface{}  `json:"message"`
	MessageSeq     DynamicInt64 `json:"message_seq"`
	RawMessage     string       `json:"raw_message"`
	Echo           string       `json:"echo,omitempty"`
}

func (d *DynamicInt64) UnmarshalJSON(b []byte) error {
	var n int64
	var s string

	// 首先尝试解析为字符串
	if err := json.Unmarshal(b, &s); err == nil {
		// 如果解析成功，再尝试将字符串解析为int64
		n, err = strconv.ParseInt(s, 10, 64)
		if err != nil { // 如果是非数字字符串
			// 进行 md5 处理
			hashed := md5.Sum([]byte(s))
			hexString := hex.EncodeToString(hashed[:])

			// 去除字母并取前15位
			numericPart := strings.Map(func(r rune) rune {
				if unicode.IsDigit(r) {
					return r
				}
				return -1
			}, hexString)

			if len(numericPart) < 15 {
				return fmt.Errorf("哈希过短: %s", numericPart)
			}

			n, err = strconv.ParseInt(numericPart[:15], 10, 64)
			if err != nil {
				return err
			}

			// 更新映射，但限制锁的范围
			md5Int64MappingLock.Lock()
			md5Int64Mapping[n] = s
			md5Int64MappingLock.Unlock()
		}
	} else {
		// 如果字符串解析失败，再尝试解析为int64
		if err := json.Unmarshal(b, &n); err != nil {
			return err
		}
	}

	*d = DynamicInt64(n)
	return nil
}

func (d DynamicInt64) ToInt64() int64 {
	return int64(d)
}

func (d DynamicInt64) ToString() string {
	return strconv.FormatInt(int64(d), 10)
}

func originalStringFromInt64(n int64) (string, bool) {
	md5Int64MappingLock.Lock()
	defer md5Int64MappingLock.Unlock()

	originalString, exists := md5Int64Mapping[n]
	return originalString, exists
}

// 假设 wsmsg.Message 是一个字符串，包含了形如 [CQ:at=xxxx] 的数据
func extractAtElements(str string) []*message.AtElement {
	re := regexp.MustCompile(`\[CQ:at=(\d+)\]`) // 正则表达式匹配[CQ:at=xxxx]其中xxxx是数字
	matches := re.FindAllStringSubmatch(str, -1)

	var elements []*message.AtElement
	for _, match := range matches {
		if len(match) == 2 {
			target, err := strconv.ParseInt(match[1], 10, 64) // 转化xxxx为int64
			if err != nil {
				continue // 如果转化失败，跳过此次循环
			}
			elements = append(elements, &message.AtElement{
				Target: target,
				// 如果有 Display 和 SubType 的信息，也可以在这里赋值
			})
		}
	}
	return elements
}

func parseEcho(echo string) (string, string) {
	parts := strings.SplitN(echo, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return echo, ""
}

func (c *QQClient) handleConnection(ws *websocket.Conn) {
	//为结构体字段赋值 以便在其他地方访问
	c.ws = ws
	defer ws.Close()
	for {
		_, p, err := ws.ReadMessage()
		if err != nil {
			logger.Error(err)
			//log.Println(err)
			return
		}
		// 打印收到的消息
		//log.Println(string(p))
		logger.Debugf("Received message: %s", string(p))
		//初步解析
		var basicMsg BasicMessage
		err = json.Unmarshal(p, &basicMsg)
		if err != nil {
			//log.Println("Failed to parse basic message:", err)
			logger.Errorf("Failed to parse basic message: %v", err)
			continue
		}
		respCh, isResponse := c.responseChans[basicMsg.Echo]
		respCha, isResponseA := c.responseMembers[basicMsg.Echo]
		//根据echo判断
		if isResponse || isResponseA {
			action, _ := parseEcho(basicMsg.Echo)
			logger.Debug(action)
			switch action {
			case "get_group_list":
				var groupData ResponseGroupData
				if err := json.Unmarshal(basicMsg.Data, &groupData.Data); err != nil {
					//log.Println("Failed to unmarshal group data:", err)
					logger.Errorf("Failed to unmarshal group data: %v", err)
					continue
				}
				respCh <- &groupData
			case "get_group_member_list":

				var memberData ResponseGroupMemberData
				if err := json.Unmarshal(basicMsg.Data, &memberData.Data); err != nil {
					//log.Println("Failed to unmarshal group member data:", err)
					logger.Errorf("Failed to unmarshal group member data: %v", err)
					continue
				}
				respCha <- &memberData
			}
			//其他类型
			delete(c.responseChans, basicMsg.Echo)
			continue
		}
		// 解析消息
		var wsmsg WebSocketMessage
		err = json.Unmarshal(p, &wsmsg)
		if err != nil {
			//log.Println("Failed to parse message:", err)
			logger.Errorf("Failed to parse message: %v", err)
			continue
		}
		// 存储 echo
		c.currentEcho = wsmsg.Echo
		// 变更UIN
		c.Uin = int64(wsmsg.SelfID)
		// 处理解析后的消息
		if wsmsg.MessageType == "group" {
			var groupName = ""
			if len(c.GroupList) > 0 {
				groupName = c.FindGroupByUin(wsmsg.GroupID.ToInt64()).Name
			}
			g := &message.GroupMessage{
				Id:        int32(wsmsg.MessageSeq),
				GroupCode: wsmsg.GroupID.ToInt64(),
				GroupName: groupName,
				Sender: &message.Sender{
					Uin:      wsmsg.Sender.UserID.ToInt64(),
					Nickname: wsmsg.Sender.Nickname,
					CardName: wsmsg.Sender.Card,
					IsFriend: false,
				},
				Time:           int32(wsmsg.Time),
				OriginalObject: nil,
			}

			if MessageContent, ok := wsmsg.MessageContent.(string); ok {
				// 替换字符串中的"\/"为"/"
				MessageContent = strings.Replace(MessageContent, "\\/", "/", -1)
				// 使用extractAtElements函数从wsmsg.Message中提取At元素
				atElements := extractAtElements(MessageContent)
				// 将提取的At元素和文本元素都添加到g.Elements
				g.Elements = append(g.Elements, &message.TextElement{Content: MessageContent})
				for _, elem := range atElements {
					g.Elements = append(g.Elements, elem)
				}
			} else if contentArray, ok := wsmsg.MessageContent.([]interface{}); ok {
				for _, contentInterface := range contentArray {
					contentMap, ok := contentInterface.(map[string]interface{})
					if !ok {
						continue
					}

					contentType, ok := contentMap["type"].(string)
					if !ok {
						continue
					}

					switch contentType {
					case "text":
						text, ok := contentMap["data"].(map[string]interface{})["text"].(string)
						if ok {
							// 替换字符串中的"\/"为"/"
							text = strings.Replace(text, "\\/", "/", -1)
							g.Elements = append(g.Elements, &message.TextElement{Content: text})
						}
					case "at":
						if data, ok := contentMap["data"].(map[string]interface{}); ok {
							var qq int
							if qqData, ok := data["qq"].(string); ok {
								if qqData != "all" {
									qq, err = strconv.Atoi(qqData)
									if err != nil {
										logger.Errorf("Failed to parse qq: %v", err)
										continue
									}
								} else {
									qq = 0
								}
								//atText := fmt.Sprintf("[CQ:at,qq=%s]", qq)
								g.Elements = append(g.Elements, &message.AtElement{Target: int64(qq), Display: g.Sender.DisplayName()})
							}
						}
					}
				}
			}
			logger.Debugf("准备c.GroupMessageEvent.dispatch(c, g)")
			//fmt.Println("准备c.GroupMessageEvent.dispatch(c, g)")
			logger.Infof("%+v", g)
			//fmt.Printf("%+v\n", g)
			// 使用 dispatch 方法
			c.GroupMessageEvent.dispatch(c, g)
		}

		if wsmsg.MessageType == "private" {
			pMsg := &message.PrivateMessage{
				Id:         int32(wsmsg.MessageID),
				InternalId: 0,
				Self:       c.Uin,
				Target:     wsmsg.Sender.UserID.ToInt64(),
				Time:       int32(wsmsg.Time),
				Sender: &message.Sender{
					Uin:      wsmsg.Sender.UserID.ToInt64(),
					Nickname: wsmsg.Sender.Nickname,
					CardName: "", // Private message might not have a Card
					IsFriend: true,
				},
			}

			if MessageContent, ok := wsmsg.MessageContent.(string); ok {
				// 替换字符串中的"\/"为"/"
				MessageContent = strings.Replace(MessageContent, "\\/", "/", -1)
				// 使用extractAtElements函数从wsmsg.Message中提取At元素
				atElements := extractAtElements(MessageContent)
				// 将提取的At元素和文本元素都添加到g.Elements
				pMsg.Elements = append(pMsg.Elements, &message.TextElement{Content: MessageContent})
				for _, elem := range atElements {
					pMsg.Elements = append(pMsg.Elements, elem)
				}
			} else if contentArray, ok := wsmsg.MessageContent.([]interface{}); ok {
				for _, contentInterface := range contentArray {
					contentMap, ok := contentInterface.(map[string]interface{})
					if !ok {
						continue
					}

					contentType, ok := contentMap["type"].(string)
					if !ok {
						continue
					}

					switch contentType {
					case "text":
						text, ok := contentMap["data"].(map[string]interface{})["text"].(string)
						if ok {
							// 替换字符串中的"\/"为"/"
							text = strings.Replace(text, "\\/", "/", -1)
							pMsg.Elements = append(pMsg.Elements, &message.TextElement{Content: text})
						}
					case "at":
						if data, ok := contentMap["data"].(map[string]interface{}); ok {
							var qq int
							if qqData, ok := data["qq"].(string); ok {
								if qqData != "all" {
									qq, err = strconv.Atoi(qqData)
									if err != nil {
										logger.Errorf("Failed to parse qq: %v", err)
										continue
									}
								} else {
									qq = 0
								}
								//atText := fmt.Sprintf("[CQ:at,qq=%s]", qq)
								pMsg.Elements = append(pMsg.Elements, &message.AtElement{Target: int64(qq), Display: pMsg.Sender.DisplayName()})
							}
						}
					}
				}
			}
			selfIDStr := strconv.FormatInt(int64(wsmsg.SelfID), 10)
			if selfIDStr == strconv.FormatInt(int64(wsmsg.Sender.UserID), 10) {
				logger.Debugf("准备c.SelfPrivateMessageEvent.dispatch(c, pMsg)")
				c.SelfPrivateMessageEvent.dispatch(c, pMsg)
			} else {
				logger.Debugf("准备c.PrivateMessageEvent.dispatch(c, pMsg)")
				c.PrivateMessageEvent.dispatch(c, pMsg)
			}

			logger.Infof("%+v", pMsg)
		}
	}
}

func (c *QQClient) StartWebSocketServer() {
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			//log.Println(err)
			logger.Error(err)
			return
		}

		// 打印新的 WebSocket 连接日志
		logger.Info("有新的ws连接了!!")
		//log.Println("有新的ws连接了!!")

		// 打印客户端的 headers
		for name, values := range r.Header {
			for _, value := range values {
				logger.WithField(name, value).Debug()
				//log.Printf("%s: %s", name, value)
			}
		}

		c.handleConnection(ws)
	})

	ws_addr := config.GlobalConfig.GetString("ws-server")
	if ws_addr == "" {
		ws_addr = "0.0.0.0:15630"
	}
	logger.WithField("force", true).Printf("WebSocket server started on ws://%s", ws_addr)
	logger.Fatal(http.ListenAndServe(ws_addr, nil))
	//log.Println("WebSocket server started on ws://0.0.0.0:15630")
	//log.Fatal(http.ListenAndServe("0.0.0.0:15630", nil))
}

func (c *QQClient) sendToWebSocketClient(ws *websocket.Conn, message []byte) {
	if ws != nil {
		err := ws.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			//log.Printf("Failed to send message to WebSocket client: %v", err)
			logger.Errorf("Failed to send message to WebSocket client: %v", err)
		}
	}
}

// NewClient create new qq client
func NewClient(uin int64, password string) *QQClient {
	return NewClientMd5(uin, md5.Sum([]byte(password)))
}

func NewClientEmpty() *QQClient {
	return NewClientMd5(0, [16]byte{})
}

func NewClientMd5(uin int64, passwordMd5 [16]byte) *QQClient {
	cli := &QQClient{
		Uin:         uin,
		PasswordMd5: passwordMd5,
		AllowSlider: true,
		TCP:         &network.TCPClient{},
		sig: &auth.SigInfo{
			OutPacketSessionID: []byte{0x02, 0xB0, 0x5B, 0x8B},
		},
		msgSvcCache:     utils.NewCache[unit](time.Second * 15),
		transCache:      utils.NewCache[unit](time.Second * 15),
		onlinePushCache: utils.NewCache[unit](time.Second * 15),
		alive:           true,
		highwaySession:  new(highway.Session),
	}

	go cli.StartWebSocketServer() // 异步启动WebSocket服务端

	cli.transport = &network.Transport{Sig: cli.sig}
	cli.oicq = oicq.NewCodec(cli.Uin)
	{ // init atomic values
		cli.SequenceId.Store(int32(rand.Intn(100000)))
		cli.requestPacketRequestID.Store(1921334513)
		cli.groupSeq.Store(int32(rand.Intn(20000)))
		cli.friendSeq.Store(22911)
		cli.highwayApplyUpSeq.Store(77918)
	}
	cli.highwaySession.Uin = strconv.FormatInt(cli.Uin, 10)
	cli.GuildService = &GuildService{c: cli}
	cli.TCP.PlannedDisconnect(cli.plannedDisconnect)
	cli.TCP.UnexpectedDisconnect(cli.unexpectedDisconnect)
	return cli
}

func (c *QQClient) version() *auth.AppVersion {
	return c.transport.Version
}

func (c *QQClient) Device() *DeviceInfo {
	return c.transport.Device
}

func (c *QQClient) UseDevice(info *auth.Device) {
	c.transport.Version = info.Protocol.Version()
	c.transport.Device = info
	c.highwaySession.AppID = int32(c.version().AppId)
	c.sig.Ksid = []byte(fmt.Sprintf("|%s|A8.2.7.27f6ea96", info.IMEI))
}

func (c *QQClient) Release() {
	if c.Online.Load() {
		c.Disconnect()
	}
	c.alive = false
}

// Login send login request
func (c *QQClient) Login() (*LoginResponse, error) {
	if c.Online.Load() {
		return nil, ErrAlreadyOnline
	}
	err := c.connect()
	if err != nil {
		return nil, err
	}
	rsp, err := c.sendAndWait(c.buildLoginPacket())
	if err != nil {
		c.Disconnect()
		return nil, err
	}
	l := rsp.(LoginResponse)
	if l.Success {
		err = c.init(false)
	}
	return &l, err
}

func (c *QQClient) TokenLogin(token []byte) error {
	if c.Online.Load() {
		return ErrAlreadyOnline
	}
	err := c.connect()
	if err != nil {
		return err
	}
	{
		r := binary.NewReader(token)
		c.Uin = r.ReadInt64()
		c.sig.D2 = r.ReadBytesShort()
		c.sig.D2Key = r.ReadBytesShort()
		c.sig.TGT = r.ReadBytesShort()
		c.sig.SrmToken = r.ReadBytesShort()
		c.sig.T133 = r.ReadBytesShort()
		c.sig.EncryptedA1 = r.ReadBytesShort()
		c.oicq.WtSessionTicketKey = r.ReadBytesShort()
		c.sig.OutPacketSessionID = r.ReadBytesShort()
		// SystemDeviceInfo.TgtgtKey = r.ReadBytesShort()
		c.Device().TgtgtKey = r.ReadBytesShort()
	}
	_, err = c.sendAndWait(c.buildRequestChangeSigPacket(true))
	if err != nil {
		return err
	}
	return c.init(true)
}

func (c *QQClient) FetchQRCode() (*QRCodeLoginResponse, error) {
	return c.FetchQRCodeCustomSize(3, 4, 2)
}

func (c *QQClient) FetchQRCodeCustomSize(size, margin, ecLevel uint32) (*QRCodeLoginResponse, error) {
	if c.Online.Load() {
		return nil, ErrAlreadyOnline
	}
	err := c.connect()
	if err != nil {
		return nil, err
	}
	i, err := c.sendAndWait(c.buildQRCodeFetchRequestPacket(size, margin, ecLevel))
	if err != nil {
		return nil, errors.Wrap(err, "fetch qrcode error")
	}
	return i.(*QRCodeLoginResponse), nil
}

func (c *QQClient) QueryQRCodeStatus(sig []byte) (*QRCodeLoginResponse, error) {
	i, err := c.sendAndWait(c.buildQRCodeResultQueryRequestPacket(sig))
	if err != nil {
		return nil, errors.Wrap(err, "query result error")
	}
	return i.(*QRCodeLoginResponse), nil
}

func (c *QQClient) QRCodeLogin(info *QRCodeLoginInfo) (*LoginResponse, error) {
	i, err := c.sendAndWait(c.buildQRCodeLoginPacket(info.tmpPwd, info.tmpNoPicSig, info.tgtQR))
	if err != nil {
		return nil, errors.Wrap(err, "qrcode login error")
	}
	rsp := i.(LoginResponse)
	if rsp.Success {
		err = c.init(false)
	}
	return &rsp, err
}

// SubmitCaptcha send captcha to server
func (c *QQClient) SubmitCaptcha(result string, sign []byte) (*LoginResponse, error) {
	seq, packet := c.buildCaptchaPacket(result, sign)
	rsp, err := c.sendAndWait(seq, packet)
	if err != nil {
		c.Disconnect()
		return nil, err
	}
	l := rsp.(LoginResponse)
	if l.Success {
		err = c.init(false)
	}
	return &l, err
}

func (c *QQClient) SubmitTicket(ticket string) (*LoginResponse, error) {
	seq, packet := c.buildTicketSubmitPacket(ticket)
	rsp, err := c.sendAndWait(seq, packet)
	if err != nil {
		c.Disconnect()
		return nil, err
	}
	l := rsp.(LoginResponse)
	if l.Success {
		err = c.init(false)
	}
	return &l, err
}

func (c *QQClient) SubmitSMS(code string) (*LoginResponse, error) {
	rsp, err := c.sendAndWait(c.buildSMSCodeSubmitPacket(code))
	if err != nil {
		c.Disconnect()
		return nil, err
	}
	l := rsp.(LoginResponse)
	if l.Success {
		err = c.init(false)
	}
	return &l, err
}

func (c *QQClient) RequestSMS() bool {
	rsp, err := c.sendAndWait(c.buildSMSRequestPacket())
	if err != nil {
		c.error("request sms error: %v", err)
		return false
	}
	return rsp.(LoginResponse).Error == SMSNeededError
}

func (c *QQClient) init(tokenLogin bool) error {
	//新增
	c.responseChans = make(map[string]chan *ResponseGroupData)
	if len(c.sig.G) == 0 {
		c.warning("device lock is disabled. HTTP API may fail.")
	}
	c.highwaySession.Uin = strconv.FormatInt(c.Uin, 10)
	if err := c.registerClient(); err != nil {
		return errors.Wrap(err, "register error")
	}
	if tokenLogin {
		notify := make(chan struct{}, 2)
		d := c.waitPacket("StatSvc.ReqMSFOffline", func(i any, err error) {
			notify <- struct{}{}
		})
		d2 := c.waitPacket("MessageSvc.PushForceOffline", func(i any, err error) {
			notify <- struct{}{}
		})
		select {
		case <-notify:
			d()
			d2()
			return errors.New("token failed")
		case <-time.After(time.Second):
			d()
			d2()
		}
	}
	c.groupSysMsgCache, _ = c.GetGroupSystemMessages()
	if !c.heartbeatEnabled {
		go c.doHeartbeat()
	}
	_ = c.RefreshStatus()
	if c.version().Protocol == auth.QiDian {
		_, _ = c.sendAndWait(c.buildLoginExtraPacket())     // 小登录
		_, _ = c.sendAndWait(c.buildConnKeyRequestPacket()) // big data key 如果等待 config push 的话时间来不及
	}
	seq, pkt := c.buildGetMessageRequestPacket(msg.SyncFlag_START, time.Now().Unix())
	_, _ = c.sendAndWait(seq, pkt, network.RequestParams{"used_reg_proxy": true, "init": true})
	c.syncChannelFirstView()
	return nil
}

func (c *QQClient) GenToken() []byte {
	return binary.NewWriterF(func(w *binary.Writer) {
		w.WriteUInt64(uint64(c.Uin))
		w.WriteBytesShort(c.sig.D2)
		w.WriteBytesShort(c.sig.D2Key)
		w.WriteBytesShort(c.sig.TGT)
		w.WriteBytesShort(c.sig.SrmToken)
		w.WriteBytesShort(c.sig.T133)
		w.WriteBytesShort(c.sig.EncryptedA1)
		w.WriteBytesShort(c.oicq.WtSessionTicketKey)
		w.WriteBytesShort(c.sig.OutPacketSessionID)
		w.WriteBytesShort(c.Device().TgtgtKey)
	})
}

func (c *QQClient) SetOnlineStatus(s UserOnlineStatus) {
	if s < 1000 {
		_, _ = c.sendAndWait(c.buildStatusSetPacket(int32(s), 0))
		return
	}
	_, _ = c.sendAndWait(c.buildStatusSetPacket(11, int32(s)))
}

func (c *QQClient) GetWordSegmentation(text string) ([]string, error) {
	rsp, err := c.sendAndWait(c.buildWordSegmentationPacket([]byte(text)))
	if err != nil {
		return nil, err
	}
	if data, ok := rsp.([][]byte); ok {
		var ret []string
		for _, val := range data {
			ret = append(ret, string(val))
		}
		return ret, nil
	}
	return nil, errors.New("decode error")
}

func (c *QQClient) GetSummaryInfo(target int64) (*SummaryCardInfo, error) {
	rsp, err := c.sendAndWait(c.buildSummaryCardRequestPacket(target))
	if err != nil {
		return nil, err
	}
	return rsp.(*SummaryCardInfo), nil
}

// ReloadFriendList refresh QQClient.FriendList field via GetFriendList()
func (c *QQClient) ReloadFriendList() error {
	rsp, err := c.GetFriendList()
	if err != nil {
		return err
	}
	c.FriendList = rsp.List
	return nil
}

// GetFriendList
// 当使用普通QQ时: 请求好友列表
// 当使用企点QQ时: 请求外部联系人列表
func (c *QQClient) GetFriendList() (*FriendListResponse, error) {
	if c.version().Protocol == auth.QiDian {
		rsp, err := c.getQiDianAddressDetailList()
		if err != nil {
			return nil, err
		}
		return &FriendListResponse{TotalCount: int32(len(rsp)), List: rsp}, nil
	}
	curFriendCount := 0
	r := &FriendListResponse{}
	for {
		rsp, err := c.sendAndWait(c.buildFriendGroupListRequestPacket(int16(curFriendCount), 150, 0, 0))
		if err != nil {
			return nil, err
		}
		list := rsp.(*FriendListResponse)
		r.TotalCount = list.TotalCount
		r.List = append(r.List, list.List...)
		curFriendCount += len(list.List)
		if int32(len(r.List)) >= r.TotalCount {
			break
		}
	}
	return r, nil
}

func (c *QQClient) SendGroupPoke(groupCode, target int64) {
	_, _ = c.sendAndWait(c.buildGroupPokePacket(groupCode, target))
}

func (c *QQClient) SendFriendPoke(target int64) {
	_, _ = c.sendAndWait(c.buildFriendPokePacket(target))
}

func (c *QQClient) ReloadGroupList() error {
	c.groupListLock.Lock()
	defer c.groupListLock.Unlock()
	list, err := c.GetGroupList()
	if err != nil {
		return err
	}
	c.GroupList = list
	return nil
}

func generateEcho(action string) string {
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s:%d", action, timestamp)
}

func (c *QQClient) GetGroupList() ([]*GroupInfo, error) {
	echo := generateEcho("get_group_list")
	//botqq := config.GlobalConfig.GetString("botqq")
	// 构建请求
	req := map[string]interface{}{
		"action": "get_group_list",
		"params": map[string]string{
			//"botqq": botqq,
		},
		"echo": echo,
	}
	// 创建响应通道并添加到映射中
	respChan := make(chan *ResponseGroupData)
	// 初始化 c.responseChans 映射
	c.responseChans = make(map[string]chan *ResponseGroupData)
	c.responseChans[echo] = respChan

	// 发送请求
	data, _ := json.Marshal(req)
	c.sendToWebSocketClient(c.ws, data)

	// 等待响应或超时
	select {
	case resp := <-respChan:
		// 根据resp的结构处理数据
		//if resp.Retcode != 0 || resp.Status != "ok" {
		//	return nil, fmt.Errorf("error response from server: %s", resp.Message)
		//}

		// 将ResponseGroupData转换为GroupInfo列表
		groups := make([]*GroupInfo, len(resp.Data))
		c.debug("GetGroupList: %v", resp.Data)
		for i, groupData := range resp.Data {
			groups[i] = &GroupInfo{
				Uin:             groupData.GroupID,
				Code:            groupData.GroupID,
				Name:            groupData.GroupName,
				GroupCreateTime: uint32(groupData.GroupCreateTime),
				GroupLevel:      uint32(groupData.GroupLevel),
				MemberCount:     uint16(groupData.MemberCount),
				MaxMemberCount:  uint16(groupData.MaxMemberCount),
				// TODO: add more fields if necessary, like Members, etc.
			}
		}

		return groups, nil

	case <-time.After(10 * time.Second):
		return nil, errors.New("GetGroupList: timeout waiting for response.")
	}

	// TODO:
	// 返回GroupInfo结构体列表

	// rsp, err := c.sendAndWait(c.buildGroupListRequestPacket(EmptyBytes))
	// if err != nil {
	// 	return nil, err
	// }
	// interner := intern.NewStringInterner()
	// r := rsp.([]*GroupInfo)
	// wg := sync.WaitGroup{}
	// batch := 50
	// for i := 0; i < len(r); i += batch {
	// 	k := i + batch
	// 	if k > len(r) {
	// 		k = len(r)
	// 	}
	// 	wg.Add(k - i)
	// 	for j := i; j < k; j++ {
	// 		go func(g *GroupInfo, wg *sync.WaitGroup) {
	// 			defer wg.Done()
	// 			m, err := c.getGroupMembers(g, interner)
	// 			if err != nil {
	// 				return
	// 			}
	// 			g.Members = m
	// 			g.Name = interner.Intern(g.Name)
	// 		}(r[j], &wg)
	// 	}
	// 	wg.Wait()
	// }
	// return r, nil

	//虚拟数据
	// logger.WithField("force", true).Print("欺骗ddbot,给它假的群数据")
	// fmt.Printf("欺骗ddbot,给它假的群数据")
	// groups := []*GroupInfo{
	// 	{
	// 		Uin:             670078416,
	// 		Code:            670078416,
	// 		Name:            "TestGroup",
	// 		OwnerUin:        2022717137,
	// 		GroupCreateTime: 1634000000,
	// 		GroupLevel:      1,
	// 		MemberCount:     3,
	// 		MaxMemberCount:  100,
	// 		Members: []*GroupMemberInfo{
	// 			{
	// 				Uin:             12345678,
	// 				Nickname:        "Member1",
	// 				CardName:        "Card1",
	// 				JoinTime:        1633900000,
	// 				LastSpeakTime:   1633999999,
	// 				SpecialTitle:    "Title1",
	// 				ShutUpTimestamp: 0,
	// 				Permission:      0,
	// 				Level:           1,
	// 				Gender:          1,
	// 			},
	// 			{
	// 				Uin:             2022717137,
	// 				Nickname:        "Owner",
	// 				CardName:        "OwnerCard",
	// 				JoinTime:        1633700000,
	// 				LastSpeakTime:   1633799999,
	// 				SpecialTitle:    "OwnerTitle",
	// 				ShutUpTimestamp: 0,
	// 				Permission:      1,
	// 				Level:           3,
	// 				Gender:          1,
	// 			},
	// 		},
	// 		LastMsgSeq: 10000,
	// 		client:     c,
	// 	},
	// }
	// return groups, nil
}

func (c *QQClient) GetGroupMembers(group *GroupInfo) ([]*GroupMemberInfo, error) {
	interner := intern.NewStringInterner()
	return c.getGroupMembers(group, interner)
}

func (c *QQClient) getGroupMembers(group *GroupInfo, interner *intern.StringInterner) ([]*GroupMemberInfo, error) {
	// var nextUin int64
	//var list []*GroupMemberInfo

	echo := generateEcho("get_group_member_list")
	// 构建请求
	req := map[string]interface{}{
		"action": "get_group_member_list",
		"params": map[string]string{
			"group_id": strconv.FormatInt(group.Uin, 10),
		},
		"echo": echo,
	}
	// 创建响应通道并添加到映射中
	respChan := make(chan *ResponseGroupMemberData)
	// 初始化 c.responseChans 映射
	c.responseMembers = make(map[string]chan *ResponseGroupMemberData)
	c.responseMembers[echo] = respChan

	// 发送请求
	data, _ := json.Marshal(req)
	c.sendToWebSocketClient(c.ws, data)

	// 等待响应或超时
	select {
	case resp := <-respChan:
		// 根据resp的结构处理数据
		//if resp.Retcode != 0 || resp.Status != "ok" {
		//	return nil, fmt.Errorf("error response from server: %s", resp.Message)
		//}

		// 将ResponseGroupMemberData转换为GroupMemberInfo列表
		members := make([]*GroupMemberInfo, len(resp.Data))
		c.debug("GetGroupMembers: %v", resp.Data)
		for i, memberData := range resp.Data {
			var permission MemberPermission
			if memberData.Role == "owner" {
				permission = 1
			} else if memberData.Role == "admin" {
				permission = 2
			} else if memberData.Role == "member" {
				permission = 3
			}
			members[i] = &GroupMemberInfo{
				Group:           group,
				Uin:             memberData.UserID,
				Nickname:        memberData.Nickname,
				CardName:        memberData.Card,
				JoinTime:        memberData.JoinTime,
				LastSpeakTime:   memberData.LastSentTime,
				SpecialTitle:    memberData.Title,
				ShutUpTimestamp: memberData.ShutUpTimestamp,
				Permission:      permission,
			}
		}

		return members, nil

	case <-time.After(10 * time.Second):
		return nil, errors.New("GetGroupMembers: timeout waiting for response.")
	}

	// for {
	// 	data, err := c.sendAndWait(c.buildGroupMemberListRequestPacket(group.Uin, group.Code, nextUin))
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	if data == nil {
	// 		return nil, errors.New("group members list is unavailable: rsp is nil")
	// 	}
	// 	rsp := data.(*groupMemberListResponse)
	// 	nextUin = rsp.NextUin
	// 	for _, m := range rsp.list {
	// 		m.Group = group
	// 		if m.Uin == group.OwnerUin {
	// 			m.Permission = Owner
	// 		}
	// 		m.CardName = interner.Intern(m.CardName)
	// 		m.Nickname = interner.Intern(m.Nickname)
	// 		m.SpecialTitle = interner.Intern(m.SpecialTitle)
	// 	}
	// 	list = append(list, rsp.list...)
	// 	if nextUin == 0 {
	// 		sort.Slice(list, func(i, j int) bool {
	// 			return list[i].Uin < list[j].Uin
	// 		})
	// 		return list, nil
	// 	}
	// }
}

func (c *QQClient) GetMemberInfo(groupCode, memberUin int64) (*GroupMemberInfo, error) {
	info, err := c.sendAndWait(c.buildGroupMemberInfoRequestPacket(groupCode, memberUin))
	if err != nil {
		return nil, err
	}
	return info.(*GroupMemberInfo), nil
}

func (c *QQClient) FindFriend(uin int64) *FriendInfo {
	if uin == c.Uin {
		return &FriendInfo{
			Uin:      uin,
			Nickname: c.Nickname,
		}
	}
	for _, t := range c.FriendList {
		f := t
		if f.Uin == uin {
			return f
		}
	}
	return nil
}

func (c *QQClient) DeleteFriend(uin int64) error {
	if c.FindFriend(uin) == nil {
		return errors.New("friend not found")
	}
	_, err := c.sendAndWait(c.buildFriendDeletePacket(uin))
	return errors.Wrap(err, "delete friend error")
}

func (c *QQClient) FindGroupByUin(uin int64) *GroupInfo {
	for _, g := range c.GroupList {
		f := g
		if f.Uin == uin {
			return f
		}
	}
	return nil
}

func (c *QQClient) FindGroup(code int64) *GroupInfo {
	for _, g := range c.GroupList {
		if g.Code == code {
			return g
		}
	}
	return nil
}

func (c *QQClient) SolveGroupJoinRequest(i any, accept, block bool, reason string) {
	if accept {
		block = false
		reason = ""
	}

	switch req := i.(type) {
	case *UserJoinGroupRequest:
		_, pkt := c.buildSystemMsgGroupActionPacket(req.RequestId, req.RequesterUin, req.GroupCode, func() int32 {
			if req.Suspicious {
				return 2
			} else {
				return 1
			}
		}(), false, accept, block, reason)
		_ = c.sendPacket(pkt)
	case *GroupInvitedRequest:
		_, pkt := c.buildSystemMsgGroupActionPacket(req.RequestId, req.InvitorUin, req.GroupCode, 1, true, accept, block, reason)
		_ = c.sendPacket(pkt)
	}
}

func (c *QQClient) SolveFriendRequest(req *NewFriendRequest, accept bool) {
	_, pkt := c.buildSystemMsgFriendActionPacket(req.RequestId, req.RequesterUin, accept)
	_ = c.sendPacket(pkt)
}

func (c *QQClient) getSKey() string {
	if c.sig.SKeyExpiredTime < time.Now().Unix() && len(c.sig.G) > 0 {
		c.debug("skey expired. refresh...")
		_, _ = c.sendAndWait(c.buildRequestTgtgtNopicsigPacket())
	}
	return string(c.sig.SKey)
}

func (c *QQClient) getCookies() string {
	return fmt.Sprintf("uin=o%d; skey=%s;", c.Uin, c.getSKey())
}

func (c *QQClient) getCookiesWithDomain(domain string) string {
	cookie := c.getCookies()

	if psKey, ok := c.sig.PsKeyMap[domain]; ok {
		return fmt.Sprintf("%s p_uin=o%d; p_skey=%s;", cookie, c.Uin, psKey)
	} else {
		return cookie
	}
}

func (c *QQClient) getCSRFToken() int {
	accu := 5381
	for _, b := range []byte(c.getSKey()) {
		accu = accu + (accu << 5) + int(b)
	}
	return 2147483647 & accu
}

func (c *QQClient) editMemberCard(groupCode, memberUin int64, card string) {
	_, _ = c.sendAndWait(c.buildEditGroupTagPacket(groupCode, memberUin, card))
}

func (c *QQClient) editMemberSpecialTitle(groupCode, memberUin int64, title string) {
	_, _ = c.sendAndWait(c.buildEditSpecialTitlePacket(groupCode, memberUin, title))
}

func (c *QQClient) setGroupAdmin(groupCode, memberUin int64, flag bool) {
	_, _ = c.sendAndWait(c.buildGroupAdminSetPacket(groupCode, memberUin, flag))
}

func (c *QQClient) updateGroupName(groupCode int64, newName string) {
	_, _ = c.sendAndWait(c.buildGroupNameUpdatePacket(groupCode, newName))
}

func (c *QQClient) groupMuteAll(groupCode int64, mute bool) {
	_, _ = c.sendAndWait(c.buildGroupMuteAllPacket(groupCode, mute))
}

func (c *QQClient) groupMute(groupCode, memberUin int64, time uint32) {
	_, _ = c.sendAndWait(c.buildGroupMutePacket(groupCode, memberUin, time))
}

func (c *QQClient) quitGroup(groupCode int64) {
	_, _ = c.sendAndWait(c.buildQuitGroupPacket(groupCode))
}

func (c *QQClient) KickGroupMembers(groupCode int64, msg string, block bool, memberUins ...int64) {
	_, _ = c.sendAndWait(c.buildGroupKickPacket(groupCode, msg, block, memberUins...))
}

func (g *GroupInfo) removeMember(uin int64) {
	g.Update(func(info *GroupInfo) {
		i := sort.Search(len(info.Members), func(i int) bool {
			return info.Members[i].Uin >= uin
		})
		if i >= len(info.Members) || info.Members[i].Uin != uin { // not found
			return
		}
		info.Members = append(info.Members[:i], info.Members[i+1:]...)
	})
}

func (c *QQClient) setGroupAnonymous(groupCode int64, enable bool) {
	_, _ = c.sendAndWait(c.buildSetGroupAnonymous(groupCode, enable))
}

// UpdateProfile 修改个人资料
func (c *QQClient) UpdateProfile(profile ProfileDetailUpdate) {
	_, _ = c.sendAndWait(c.buildUpdateProfileDetailPacket(profile))
}

func (c *QQClient) SetCustomServer(servers []netip.AddrPort) {
	c.servers = append(servers, c.servers...)
}

func (c *QQClient) registerClient() error {
	_, err := c.sendAndWait(c.buildClientRegisterPacket())
	if err == nil {
		c.Online.Store(true)
	}
	return err
}

func (c *QQClient) nextSeq() uint16 {
	seq := c.SequenceId.Add(1)
	if seq > 1000000 {
		seq = int32(rand.Intn(100000)) + 60000
		c.SequenceId.Store(seq)
	}
	return uint16(seq)
}

func (c *QQClient) nextPacketSeq() int32 {
	return c.requestPacketRequestID.Add(2)
}

func (c *QQClient) nextGroupSeq() int32 {
	return c.groupSeq.Add(2)
}

func (c *QQClient) nextFriendSeq() int32 {
	return c.friendSeq.Add(1)
}

func (c *QQClient) nextQWebSeq() int64 {
	return c.qwebSeq.Add(1)
}

func (c *QQClient) nextHighwayApplySeq() int32 {
	return c.highwayApplyUpSeq.Add(2)
}

func (c *QQClient) doHeartbeat() {
	c.heartbeatEnabled = true
	times := 0
	for c.Online.Load() {
		time.Sleep(time.Second * 30)
		seq := c.nextSeq()
		req := network.Request{
			Type:        network.RequestTypeLogin,
			EncryptType: network.EncryptTypeNoEncrypt,
			SequenceID:  int32(seq),
			Uin:         c.Uin,
			CommandName: "Heartbeat.Alive",
			Body:        EmptyBytes,
		}
		packet := c.transport.PackPacket(&req)
		_, err := c.sendAndWait(seq, packet)
		if errors.Is(err, network.ErrConnectionClosed) {
			continue
		}
		times++
		if times >= 7 {
			_ = c.registerClient()
			times = 0
		}
	}
	c.heartbeatEnabled = false
}
