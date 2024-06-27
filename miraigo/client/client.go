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
	ws                   *websocket.Conn
	responseChans        map[string]chan *ResponseGroupsData
	responseGroupChans   map[string]chan *ResponseGroupData
	responseMembers      map[string]chan *ResponseGroupMemberData
	responseFriends      map[string]chan *ResponseFriendList
	responseMessage      map[string]chan *ResponseSendMessage
	responseSetLeave     map[string]chan *ResponseSetGroupLeave
	responseStrangerInfo map[string]chan *ResponseGetStrangerInfo
	currentEcho          string
	limiterMessageSend   *RateLimiter
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
type ResponseGroupsData struct {
	Data    []GroupData `json:"data"`
	Message string      `json:"message"`
	Retcode int         `json:"retcode"`
	Status  string      `json:"status"`
	Echo    string      `json:"echo"`
}

// 新增
type ResponseGroupData struct {
	Data    GroupData `json:"data"`
	Message string    `json:"message"`
	Retcode int       `json:"retcode"`
	Status  string    `json:"status"`
	Echo    string    `json:"echo"`
}

// 好友信息请求返回
type ResponseFriendList struct {
	Status  string       `json:"status"`
	Retcode int          `json:"retcode"`
	Message string       `json:"message"`
	Data    []FriendData `json:"data"`
	Wording string       `json:"wording"`
}

// 好友信息
type FriendData struct {
	UserID   int64  `json:"user_id"`
	NickName string `json:"nick_name"`
	Remark   string `json:"remark"`
	Sex      string `json:"sex"`
	Level    int    `json:"level"`
}

// 群成员信息
type MemberData struct {
	GroupID         int64       `json:"group_id"`
	UserID          int64       `json:"user_id"`
	Nickname        string      `json:"nickname"`
	Card            string      `json:"card"`
	Sex             string      `json:"sex"`
	Age             int         `json:"age"`
	Area            string      `json:"area"`
	Level           interface{} `json:"level"`
	QQLevel         int16       `json:"qq_level"`
	JoinTime        int64       `json:"join_time"`
	LastSentTime    int64       `json:"last_sent_time"`
	TitleExpireTime int64       `json:"title_expire_time"`
	Unfriendly      bool        `json:"unfriendly"`
	CardChangeable  bool        `json:"card_changeable"`
	IsRobot         bool        `json:"is_robot"`
	ShutUpTimestamp int64       `json:"shut_up_timestamp"`
	Role            string      `json:"role"`
	Title           string      `json:"title"`
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
	Echo string `json:"echo"`
	// Data json.RawMessage `json:"data"`
}

// 发送消息返回
type ResponseSendMessage struct {
	Status  string `json:"status"`
	Retcode int    `json:"retcode"`
	Data    struct {
		MessageID int32 `json:"message_id"`
	}
	Message string `json:"message"`
	Wording string `json:"wording"`
}

// set指令返回
type ResponseSetGroupLeave struct {
	Status  string `json:"status"`
	Retcode int    `json:"retcode"`
	Message string `json:"message"`
	Wording string `json:"wording"`
	Echo    string `json:"echo"`
}

// 发送消息返回
type SendResp struct {
	RetMSG *message.GroupMessage
	Error  error
}

// 查陌生人返回
type ResponseGetStrangerInfo struct {
	Status  string `json:"status"`
	Retcode int    `json:"retcode"`
	Data    struct {
		Result int    `json:"result"`
		ErrMsg string `json:"errMsg"`
		Info   struct {
			Uin       string `json:"uin"`
			Nick      string `json:"nick"`
			LongNick  string `json:"longNick"`
			Sex       int    `json:"sex"`
			ShengXiao int    `json:"shengXiao"`
			RegTime   int64  `json:"regTime"`
			QQLevel   struct {
				CrownNum int `json:"crownNum"`
				SunNum   int `json:"sunNum"`
				MoonNum  int `json:"moonNum"`
				StarNum  int `json:"starNum"`
			} `json:"qqLevel"`
		} `json:"info"`
		UserID   int64  `json:"user_id"`
		NickName string `json:"nickname"`
		Age      int    `json:"age"`
		Level    int    `json:"level"`
	} `json:"data"`
	Message string `json:"message"`
	Wording string `json:"wording"`
	Echo    string `json:"echo"`
}

// 卡片消息
type CardMessage struct {
	App    string `json:"app"`
	Config struct {
		AutoSize int    `json:"autosize"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		Forward  int    `json:"forward"`
		CTime    int64  `json:"ctime"`
		Round    int    `json:"round"`
		Token    string `json:"token"`
		Type     string `json:"type"`
	} `json:"config"`
	Extra struct {
		AppType int   `json:"app_type"`
		AppId   int64 `json:"appid"`
		MsgSeq  int64 `json:"msg_seq"`
		Uin     int64 `json:"uin"`
	} `json:"extra"`
	Meta struct {
		News struct {
			Action         string `json:"action"`
			AndroidPkgName string `json:"android_pkg_name"`
			AppType        int    `json:"app_type"`
			AppId          int64  `json:"appid"`
			CTime          int64  `json:"ctime"`
			Desc           string `json:"desc"`
			JumpUrl        string `json:"jumpUrl"`
			PreView        string `json:"preview"`
			TagIcon        string `json:"tagIcon"`
			SourceIcon     string `json:"source_icon"`
			SourceUrl      string `json:"source_url"`
			Tag            string `json:"tag"`
			Title          string `json:"title"`
			Uin            int64  `json:"uin"`
		} `json:"news"`
		Detail_1 struct {
			AppId   string `json:"appid"`
			AppType int    `json:"app_type"`
			Title   string `json:"title"`
			Desc    string `json:"desc"`
			Icon    string `json:"icon"`
			PreView string `json:"preview"`
			Url     string `json:"url"`
			Scene   int    `json:"scene"`
			Host    struct {
				Uin  int64  `json:"uin"`
				Nick string `json:"nick"`
			} `json:"host"`
		} `json:"detail_1"`
	} `json:"meta"`
	Prompt string `json:"prompt"`
	Ver    string `json:"ver"`
	View   string `json:"view"`
}

// Window represents a fixed-window
type Window interface {
	// Start returns the start boundary
	Start() time.Time

	// Count returns the accumulated count
	Count() int64

	// AddCount increments the accumulated count by n
	AddCount(n int64)

	// Reset sets the state of the window with the given settings
	Reset(s time.Time, c int64)
}

type NewWindow func() Window

type LocalWindow struct {
	start int64
	count int64
}

type RateLimiter struct {
	size  time.Duration
	limit int64

	mu sync.Mutex

	curr Window
	prev Window
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

func NewLocalWindow() *LocalWindow {
	return &LocalWindow{}
}

func (w *LocalWindow) Start() time.Time {
	return time.Unix(0, w.start)
}

func (w *LocalWindow) Count() int64 {
	return w.count
}

func (w *LocalWindow) AddCount(n int64) {
	w.count += n
}

func (w *LocalWindow) Reset(s time.Time, c int64) {
	w.start = s.UnixNano()
	w.count = c
}

func NewRateLimiter(size time.Duration, limit int64, newWindow NewWindow) *RateLimiter {
	currWin := newWindow()

	// The previous window is static (i.e. no add changes will happen within it),
	// so we always create it as an instance of LocalWindow
	prevWin := NewLocalWindow()

	return &RateLimiter{
		size:  size,
		limit: limit,
		curr:  currWin,
		prev:  prevWin,
	}

}

// Size returns the time duration of one window size
func (rl *RateLimiter) Size() time.Duration {
	return rl.size
}

// Limit returns the maximum events permitted to happen during one window size
func (rl *RateLimiter) Limit() int64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.limit
}

func (rl *RateLimiter) SetLimit(limit int64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limit = limit
}

// shorthand for GrantN(time.Now(), 1)
func (rl *RateLimiter) Grant() bool {
	return rl.GrantN(time.Now(), 1)
}

// reports whether n events may happen at time now
func (rl *RateLimiter) GrantN(now time.Time, n int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.advance(now)

	elapsed := now.Sub(rl.curr.Start())
	weight := float64(rl.size-elapsed) / float64(rl.size)
	count := int64(weight*float64(rl.prev.Count())) + rl.curr.Count()

	if count+n > rl.limit {
		return false
	}

	rl.curr.AddCount(n)
	return true
}

// advance updates the current/previous windows resulting from the passage of time
func (rl *RateLimiter) advance(now time.Time) {
	// Calculate the start boundary of the expected current-window.
	newCurrStart := now.Truncate(rl.size)

	diffSize := newCurrStart.Sub(rl.curr.Start()) / rl.size
	if diffSize >= 1 {
		// The current-window is at least one-window-size behind the expected one.
		newPrevCount := int64(0)
		if diffSize == 1 {
			// The new previous-window will overlap with the old current-window,
			// so it inherits the count.
			newPrevCount = rl.curr.Count()
		}

		rl.prev.Reset(newCurrStart.Add(-rl.size), newPrevCount)
		// The new current-window always has zero count.
		rl.curr.Reset(newCurrStart, 0)
	}
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

type SendMsg struct {
	GroupCode  int64
	Message    *message.SendingMessage
	NewStr     string
	ResultChan chan SendResp
}

var sendMessageQueue = make(chan SendMsg, 128)
var messageQueue = make(chan []byte, 128)
var md5Int64Mapping = make(map[int64]string)
var md5Int64MappingLock sync.Mutex

type DynamicInt64 int64

type MessageContent struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

type WebSocketMessage struct {
	PostType      string       `json:"post_type"`
	MetaEventType string       `json:"meta_event_type"`
	NoticeType    string       `json:"notice_type"`
	OperatorId    DynamicInt64 `json:"operator_id"`
	// Status        struct {
	// 	Online bool `json:"online"`
	// 	Good   bool `json:"good"`
	// } `json:"status"`
	RequestType string       `json:"request_type"`
	Comment     string       `json:"comment"`
	Flag        string       `json:"flag"`
	Interval    int          `json:"interval"`
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
	RealId         DynamicInt64 `json:"real_id"`
	GroupID        DynamicInt64 `json:"group_id"`
	MessageContent interface{}  `json:"message"`
	MessageSeq     DynamicInt64 `json:"message_seq"`
	RawMessage     string       `json:"raw_message"`
	TargetID       DynamicInt64 `json:"target_id"`
	Duration       int32        `json:"duration"`
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
		messageQueue <- p
	}
}

func (c *QQClient) processMessage() {
	for {
		select {
		case p := <-messageQueue:
			go c.handleResponse(p)
		}
	}
}

func (c *QQClient) handleResponse(p []byte) {
	go logger.Tracef("Received message: %s", string(p))
	//初步解析
	var basicMsg BasicMessage
	err := json.Unmarshal(p, &basicMsg)
	if err != nil {
		//log.Println("Failed to parse basic message:", err)
		logger.Errorf("Failed to parse basic message: %v", err)
		return
	}
	if basicMsg.Echo != "" {
		action, _ := parseEcho(basicMsg.Echo)
		logger.Debugf("Received action: %s", action)
		//根据echo判断
		switch action {
		case "get_group_list":
			respCh, isResponse := c.responseChans[basicMsg.Echo]
			if !isResponse {
				logger.Warnf("No response channel for group list: %s", basicMsg.Echo)
				return
			}
			var groupData ResponseGroupsData
			if err := json.Unmarshal(p, &groupData); err != nil {
				//log.Println("Failed to unmarshal group data:", err)
				logger.Errorf("Failed to unmarshal group data: %v", err)
				return
			}
			respCh <- &groupData
		case "get_group_info":
			respCh, isResponse := c.responseGroupChans[basicMsg.Echo]
			if !isResponse {
				logger.Warnf("No response channel for group info: %s", basicMsg.Echo)
				return
			}
			var groupData ResponseGroupData
			if err := json.Unmarshal(p, &groupData); err != nil {
				logger.Errorf("Failed to unmarshal group data: %v", err)
				return
			}
			respCh <- &groupData
		case "get_group_member_list":
			respCh, isResponse := c.responseMembers[basicMsg.Echo]
			if !isResponse {
				logger.Warnf("No response channel for group member list: %s", basicMsg.Echo)
				return
			}
			var memberData ResponseGroupMemberData
			if err := json.Unmarshal(p, &memberData); err != nil {
				//log.Println("Failed to unmarshal group member data:", err)
				logger.Errorf("Failed to unmarshal group member data: %v", err)
				return
			}
			respCh <- &memberData
		case "get_friend_list":
			respCh, isResponse := c.responseFriends[basicMsg.Echo]
			if !isResponse {
				logger.Warnf("No response channel for friend list: %s", basicMsg.Echo)
				return
			}
			var friendData ResponseFriendList
			if err := json.Unmarshal(p, &friendData); err != nil {
				//log.Println("Failed to unmarshal friend data:", err)
				logger.Errorf("Failed to unmarshal friend data: %v", err)
				return
			}
			respCh <- &friendData
		case "send_group_msg":
			respCh, isResponse := c.responseMessage[basicMsg.Echo]
			if !isResponse {
				logger.Warnf("No response channel for message: %s", basicMsg.Echo)
				return
			}
			var SendResp ResponseSendMessage
			if err := json.Unmarshal(p, &SendResp); err != nil {
				//log.Println("Failed to unmarshal send message response:", err)
				logger.Errorf("Failed to unmarshal send message response: %v", err)
				return
			}
			respCh <- &SendResp
		case "set_group_leave":
			respCh, isResponse := c.responseSetLeave[basicMsg.Echo]
			if !isResponse {
				logger.Warnf("No response channel for set group leave: %s", basicMsg.Echo)
				return
			}
			var SendResp ResponseSetGroupLeave
			if err = json.Unmarshal(p, &SendResp); err != nil {
				logger.Errorf("Failed to unmarshal set group leave response: %v", err)
				return
			}
			respCh <- &SendResp
		case "get_stranger_info":
			respCh, isResponse := c.responseStrangerInfo[basicMsg.Echo]
			if !isResponse {
				logger.Warnf("No response channel for get stranger info: %s", basicMsg.Echo)
				return
			}
			var SendResp ResponseGetStrangerInfo
			if err = json.Unmarshal(p, &SendResp); err != nil {
				logger.Errorf("Failed to unmarshal get stranger info response: %v", err)
				return
			}
			respCh <- &SendResp
		case "set_friend_add_request":
			logger.Debug("Received set friend add request")
		case "set_group_add_request":
			logger.Debug("Received set group add request")
		default:
			logger.Warnf("Unknown action: %s", action)
		}
		//其他类型
		// delete(c.responseChans, basicMsg.Echo)
	} else {
		// 解析消息
		var wsmsg WebSocketMessage
		err = json.Unmarshal(p, &wsmsg)
		if err != nil {
			//log.Println("Failed to parse message:", err)
			logger.Errorf("Failed to parse message: %v", err)
			return
		}
		// 存储 echo
		// c.currentEcho = wsmsg.Echo
		c.handleMessage(wsmsg)
	}
}

func (c *QQClient) handleMessage(wsmsg WebSocketMessage) {
	var err error
	// 处理解析后的消息
	if wsmsg.MessageType == "group" {
		// var groupName string
		// if len(c.GroupList) > 0 {
		// 	groupName = c.FindGroupByUin(wsmsg.GroupID.ToInt64()).Name
		// }
		g := &message.GroupMessage{
			Id:        int32(wsmsg.MessageID),
			GroupCode: wsmsg.GroupID.ToInt64(),
			GroupName: "",
			// GroupName: groupName,
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
						var qq int64
						if qqData, ok := data["qq"].(string); ok {
							if qqData != "all" {
								qq, err = strconv.ParseInt(qqData, 10, 64)
								if err != nil {
									logger.Errorf("Failed to parse qq: %v", err)
									continue
								}
							} else {
								qq = 0
							}
						} else if qqData, ok := data["qq"].(int64); ok {
							qq = qqData
						}
						//atText := fmt.Sprintf("[CQ:at,qq=%s]", qq)
						g.Elements = append(g.Elements, &message.AtElement{Target: qq, Display: g.Sender.DisplayName()})
					}
				case "face":
					faceID := 0
					if face, ok := contentMap["data"].(map[string]interface{})["id"].(string); ok {
						faceID, _ = strconv.Atoi(face)
					} else if face, ok := contentMap["data"].(map[string]interface{})["id"].(float64); ok {
						faceID = int(face)
					}
					g.Elements = append(g.Elements, &message.FaceElement{Index: int32(faceID)})
				case "mface":
					if mface, ok := contentMap["data"].(map[string]interface{}); ok {
						tabId, _ := strconv.Atoi(mface["emoji_package_id"].(string))
						g.Elements = append(g.Elements, &message.MarketFaceElement{
							Name:       mface["summary"].(string),
							FaceId:     []byte(mface["emoji_id"].(string)),
							TabId:      int32(tabId),
							MediaType:  2,
							EncryptKey: []byte(mface["key"].(string)),
						})
					}
				case "image":
					image, ok := contentMap["data"].(map[string]interface{})
					if ok {
						size := 0
						if tmp, ok := image["file_size"].(string); ok {
							size, err = strconv.Atoi(tmp)
						}
						g.Elements = append(g.Elements, &message.GroupImageElement{
							Size: int32(size),
							Url:  image["url"].(string),
						})
					}
				case "reply":
					replySeq := 0
					if replyID, ok := contentMap["data"].(map[string]interface{})["id"].(string); ok {
						replySeq, _ = strconv.Atoi(replyID)
					} else if replyID, ok := contentMap["data"].(map[string]interface{})["id"].(float64); ok {
						replySeq = int(replyID)
					}
					g.Elements = append(g.Elements, &message.ReplyElement{
						ReplySeq: int32(replySeq),
						Sender:   g.Sender.Uin,
						GroupID:  g.GroupCode,
						Time:     g.Time,
						Elements: g.Elements,
					})
				case "record":
					record, ok := contentMap["data"].(map[string]interface{})
					if ok {
						fileSize := 0
						filePath := ""
						if size, ok := record["file_size"].(string); ok {
							fileSize, _ = strconv.Atoi(size)
						}
						if path, ok := record["path"].(string); ok {
							filePath = path
						}
						g.Elements = append(g.Elements, &message.VoiceElement{
							Name: record["file"].(string),
							Url:  filePath,
							Size: int32(fileSize),
						})
					}
				case "json":
					// 处理json消息
					if card, ok := contentMap["data"].(map[string]interface{}); ok {
						switch card["data"].(type) {
						case string:
							var j CardMessage
							err := json.Unmarshal([]byte(card["data"].(string)), &j)
							if err != nil {
								logger.Errorf("Failed to parse card message: %v", err)
								continue
							}
							tag, title, desc, text := "", "", "", ""
							if j.Meta.News.Title != "" {
								tag = j.Meta.News.Tag
								title = j.Meta.News.Title
								desc = j.Meta.News.Desc
								text = "[卡片][" + tag + "][" + title + "]" + desc
							} else if j.Meta.Detail_1.Title != "" {
								tag = j.Meta.Detail_1.Title
								desc = j.Meta.Detail_1.Desc
								text = "[卡片][" + tag + "]" + desc
							}
							g.Elements = append(g.Elements, &message.TextElement{Content: text})
						default:
							logger.Errorf("Unknown card message type: %v", card)
						}
					}
				case "file":
					file, ok := contentMap["data"].(map[string]interface{})
					if ok {
						text := "[文件]" + file["file"].(string)
						g.Elements = append(g.Elements, &message.TextElement{Content: text})
					}
				case "forward":
					logger.Infof("Received forward message: %v", contentMap)
				default:
					logger.Warnf("Unknown content type: %s", contentType)
				}
			}
		}
		//logger.Debugf("准备c.GroupMessageEvent.dispatch(c, g)")
		//fmt.Println("准备c.GroupMessageEvent.dispatch(c, g)")
		//fmt.Printf("%+v\n", g)
		// 使用 dispatch 方法
		// c.GroupMessageEvent.dispatch(c, g)
		// logger.Infof("%+v", g)
		c.waitDataAndDispatch(g)
	} else if wsmsg.MessageType == "private" {
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
				IsFriend: wsmsg.SubType == "friend",
			},
		}

		// 拿到发送自身发送的消息时修改为正确目标
		if wsmsg.PostType == "message_sent" {
			pMsg.Target = int64(wsmsg.TargetID)
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
						var qq int64
						if qqData, ok := data["qq"].(string); ok {
							if qqData != "all" {
								qq, err = strconv.ParseInt(qqData, 10, 64)
								if err != nil {
									logger.Errorf("Failed to parse qq: %v", err)
									continue
								}
							} else {
								qq = 0
							}
						} else if qqData, ok := data["qq"].(int64); ok {
							qq = qqData
						}
						//atText := fmt.Sprintf("[CQ:at,qq=%s]", qq)
						pMsg.Elements = append(pMsg.Elements, &message.AtElement{Target: int64(qq), Display: pMsg.Sender.DisplayName()})
					}
				case "face":
					faceID := 0
					if face, ok := contentMap["data"].(map[string]interface{})["id"].(string); ok {
						faceID, _ = strconv.Atoi(face)
					} else if face, ok := contentMap["data"].(map[string]interface{})["id"].(float64); ok {
						faceID = int(face)
					}
					pMsg.Elements = append(pMsg.Elements, &message.FaceElement{Index: int32(faceID)})
				case "mface":
					if mface, ok := contentMap["data"].(map[string]interface{}); ok {
						tabId, _ := strconv.Atoi(mface["emoji_package_id"].(string))
						pMsg.Elements = append(pMsg.Elements, &message.MarketFaceElement{
							Name:       mface["summary"].(string),
							FaceId:     []byte(mface["emoji_id"].(string)),
							TabId:      int32(tabId),
							MediaType:  2,
							EncryptKey: []byte(mface["key"].(string)),
						})
					}
				case "image":
					image, ok := contentMap["data"].(map[string]interface{})
					if ok {
						size := 0
						if tmp, ok := image["file_size"].(string); ok {
							size, err = strconv.Atoi(tmp)
						}
						pMsg.Elements = append(pMsg.Elements, &message.FriendImageElement{
							Size: int32(size),
							Url:  image["url"].(string),
						})
					}
				case "reply":
					replySeq := 0
					if replyID, ok := contentMap["data"].(map[string]interface{})["id"].(string); ok {
						replySeq, _ = strconv.Atoi(replyID)
					} else if replyID, ok := contentMap["data"].(map[string]interface{})["id"].(float64); ok {
						replySeq = int(replyID)
					}
					pMsg.Elements = append(pMsg.Elements, &message.ReplyElement{
						ReplySeq: int32(replySeq),
						Sender:   pMsg.Sender.Uin,
						Time:     pMsg.Time,
						Elements: pMsg.Elements,
					})
				case "record":
					record, ok := contentMap["data"].(map[string]interface{})
					if ok {
						fileSize := 0
						filePath := ""
						if size, ok := record["file_size"].(string); ok {
							fileSize, _ = strconv.Atoi(size)
						}
						if path, ok := record["path"].(string); ok {
							filePath = path
						}
						pMsg.Elements = append(pMsg.Elements, &message.VoiceElement{
							Name: record["file"].(string),
							Url:  filePath,
							Size: int32(fileSize),
						})
					}
				case "json":
					// 处理json消息
					if card, ok := contentMap["data"].(map[string]interface{}); ok {
						switch card["data"].(type) {
						case string:
							var j CardMessage
							err := json.Unmarshal([]byte(card["data"].(string)), &j)
							if err != nil {
								logger.Errorf("Failed to parse card message: %v", err)
								continue
							}
							tag, title, desc, text := "", "", "", ""
							if j.Meta.News.Title != "" {
								tag = j.Meta.News.Tag
								title = j.Meta.News.Title
								desc = j.Meta.News.Desc
								text = "[卡片][" + tag + "][" + title + "]" + desc
							} else if j.Meta.Detail_1.Title != "" {
								tag = j.Meta.Detail_1.Title
								desc = j.Meta.Detail_1.Desc
								text = "[卡片][" + tag + "]" + desc
							}
							pMsg.Elements = append(pMsg.Elements, &message.TextElement{Content: text})
						default:
							logger.Errorf("Unknown card message type: %v", card)
						}
					}
				case "file":
					file, ok := contentMap["data"].(map[string]interface{})
					if ok {
						text := "[文件]" + file["file"].(string)
						pMsg.Elements = append(pMsg.Elements, &message.TextElement{Content: text})
					}
				case "forward":
					logger.Infof("Received forward message: %v", contentMap)
				default:
					logger.Warnf("Unknown content type: %s", contentType)
				}
			}
		}
		selfIDStr := strconv.FormatInt(int64(wsmsg.SelfID), 10)
		if selfIDStr == strconv.FormatInt(int64(wsmsg.Sender.UserID), 10) {
			//logger.Debugf("准备c.SelfPrivateMessageEvent.dispatch(c, pMsg)")
			c.SelfPrivateMessageEvent.dispatch(c, pMsg)
		} else {
			//logger.Debugf("准备c.PrivateMessageEvent.dispatch(c, pMsg)")
			c.PrivateMessageEvent.dispatch(c, pMsg)
		}
		c.OutputReceivingMessage(pMsg)
	} else if wsmsg.MessageType == "" {
		if wsmsg.PostType == "meta_event" {
			// 元事件
			if wsmsg.SubType == "connect" {
				// 刷新Bot.Uin
				c.Uin = int64(wsmsg.SelfID)
			}
			logger.Debugf("收到 元事件 消息：%s: %s", wsmsg.SubType, wsmsg.MetaEventType)
		} else if wsmsg.PostType == "notice" {
			// 通知事件
			sync := false
			if wsmsg.NoticeType == "group_admin" {
				// 如果是权限变更，则仅刷新群成员列表
				sync = true
			} else if wsmsg.NoticeType == "group_increase" {
				// 根据是否与自身有关来选择性刷新群列表
				sync = true
				if wsmsg.UserID.ToInt64() == c.Uin {
					ret, err := c.GetGroupInfo(wsmsg.GroupID.ToInt64())
					if err == nil {
						skip := false
						for i, group := range c.GroupList {
							if group.Code == wsmsg.GroupID.ToInt64() {
								c.GroupList[i] = ret
								skip = true
								logger.Debugf("Group %d updated", wsmsg.GroupID.ToInt64())
								break
							}
						}
						if !skip {
							logger.Debugf("Group %d not found in GroupList, adding it", wsmsg.GroupID.ToInt64())
							c.GroupList = append(c.GroupList, ret)
						}
					} else {
						logger.Warnf("Failed to get group info: %v", err)
					}
				}
			} else if wsmsg.NoticeType == "group_decrease" {
				// 根据是否与自身有关来选择性刷新群列表
				if wsmsg.SubType == "kick_me" {
					for i, group := range c.GroupList {
						if group.Code == wsmsg.GroupID.ToInt64() {
							c.GroupList = append(c.GroupList[:i], c.GroupList[i+1:]...)
							logger.Debugf("Group %d removed from GroupList", wsmsg.GroupID.ToInt64())
							break
						}
					}
				} else {
					sync = true
				}
			} else if wsmsg.NoticeType == "friend_add" {
				c.ReloadFriendList()
				// c.SyncTickerControl(2, WebSocketMessage{}, false)
			} else if wsmsg.SubType == "poke" {
				// 戳一戳事件（返回示例）
				// {
				// 	"time": 1717917916,
				// 	"self_id": 1143469507,
				// 	"post_type": "notice",
				// 	"notice_type": "notify",
				// 	"sub_type": "poke",
				// 	"target_id": 0,
				// 	"user_id": 0,
				// 	"group_id": 615231979
				// }
			} else if wsmsg.NoticeType == "group_card" {
				//群名片修改（返回示例）
				// {
				// 	"time": 1717918741,
				// 	"self_id": 1143469507,
				// 	"post_type": "notice",
				// 	"group_id": 615231979,
				// 	"user_id": 1143469507,
				// 	"notice_type": "group_card",
				// 	"card_new": "吔屎大将军",
				// 	"card_old": "吔屎大将军1"
				// }
			} else if wsmsg.NoticeType == "group_ban" {
				c.GroupMuteEvent.dispatch(c, &GroupMuteEvent{
					GroupCode:   wsmsg.GroupID.ToInt64(),
					OperatorUin: wsmsg.OperatorId.ToInt64(),
					TargetUin:   wsmsg.UserID.ToInt64(),
					Time:        wsmsg.Duration,
				})
				//群禁言事件（返回示例）
				// {
				// 	"time": 1719116197,
				// 	"self_id": 1143469507,
				// 	"post_type": "notice",
				// 	"group_id": 864046682,
				// 	"user_id": 0,
				// 	"notice_type": "group_ban", // 解除 "lift_ban"
				// 	"operator_id": 313575803,
				// 	"duration": -1,
				// 	"sub_type": "ban"
				// }
			}
			// 选择性执行上述结果
			if sync {
				if c.GroupList != nil {
					// 调试：暂时强制刷新（refresh=true）
					//refresh = true
					c.SyncGroupMembers(wsmsg.GroupID)
					// c.SyncTickerControl(1, wsmsg, refresh)
				}
			}
			logger.Infof("收到 通知事件 消息：%s: %s", wsmsg.NoticeType, wsmsg.SubType)
		} else if wsmsg.PostType == "request" {
			const StragngerInfoErr = "Failed to get stranger info: %v"
			// 请求事件
			if wsmsg.RequestType == "friend" {
				friendRequest := NewFriendRequest{
					RequestId:     time.Now().UnixNano() / 1e6,
					Message:       wsmsg.Comment,
					RequesterUin:  wsmsg.UserID.ToInt64(),
					RequesterNick: "陌生人",
					Flag:          wsmsg.Flag,
					client:        c,
				}
				info, err := c.GetStrangerInfo(friendRequest.RequesterUin)
				if err == nil {
					friendRequest.RequesterNick = info.Data.NickName
				} else {
					logger.Warnf(StragngerInfoErr, err)
				}
				// 好友申请
				c.NewFriendRequestEvent.dispatch(c, &friendRequest)
			}
			if wsmsg.RequestType == "group" {
				if wsmsg.SubType == "add" {
					// 加群申请
					groupRequest := UserJoinGroupRequest{
						RequestId:     time.Now().UnixNano() / 1e6,
						Message:       wsmsg.Comment,
						RequesterUin:  wsmsg.UserID.ToInt64(),
						RequesterNick: "陌生人",
						GroupCode:     wsmsg.GroupID.ToInt64(),
						GroupName:     "未知",
						Flag:          wsmsg.Flag,
						client:        c,
					}
					user, err := c.GetStrangerInfo(groupRequest.RequesterUin)
					if err == nil {
						groupRequest.RequesterNick = user.Data.NickName
					} else {
						logger.Warnf(StragngerInfoErr, err)
					}
					groupInfo := c.FindGroupByUin(groupRequest.GroupCode)
					if groupInfo != nil {
						groupRequest.GroupName = groupInfo.Name
					}
					c.UserWantJoinGroupEvent.dispatch(c, &groupRequest)
				} else if wsmsg.SubType == "invite" {
					// 群邀请
					groupRequest := GroupInvitedRequest{
						RequestId:   time.Now().UnixNano() / 1e6,
						InvitorUin:  wsmsg.UserID.ToInt64(),
						InvitorNick: "陌生人",
						GroupCode:   wsmsg.GroupID.ToInt64(),
						GroupName:   "未知",
						Flag:        wsmsg.Flag,
						client:      c,
					}
					user, err := c.GetStrangerInfo(groupRequest.InvitorUin)
					if err == nil {
						groupRequest.InvitorNick = user.Data.NickName
					} else {
						logger.Warnf(StragngerInfoErr, err)
					}
					groupInfo := c.FindGroupByUin(groupRequest.GroupCode)
					if groupInfo != nil {
						groupRequest.GroupName = groupInfo.Name
					}
					c.GroupInvitedEvent.dispatch(c, &groupRequest)
				}
			}
			logger.Infof("收到 请求事件 消息: %s: %s", wsmsg.RequestType, wsmsg.SubType)
		}
	}
}

// func (c *QQClient) SyncTickerControl(m int, wsmsg WebSocketMessage, flash bool) {
// allow:
// 	if c.limiterMessageSend.Grant() {
// 		logger.Debugf("流控：允许通过")
// 		if m == 1 {
// 			c.SyncGroupMembers(wsmsg.GroupID, flash)
// 		} else if m == 2 {
// 			c.ReloadFriendList()
// 		}
// 	} else {
// 		logger.Debugf("流控：拒绝通过")
// 		time.Sleep(time.Second * 1)
// 		goto allow
// 	}
// }

func (c *QQClient) OutputReceivingMessage(Msg interface{}) {
	mode := false
	var content []message.IMessageElement
	if msg, ok := Msg.(*message.GroupMessage); ok {
		content = msg.GetElements()
	} else if msg, ok := Msg.(*message.PrivateMessage); ok {
		content = msg.GetElements()
		mode = true
	}
	var tmpText string
	for _, elem := range content {
		if text, ok := elem.(*message.TextElement); ok {
			if len(text.Content) > 75 {
				tmpText += text.Content[:75] + "..."
			} else {
				tmpText += text.Content
			}
		} else if _, ok := elem.(*message.AtElement); ok {
			tmpText += "[艾特]"
		} else if _, ok := elem.(*message.FaceElement); ok {
			tmpText += "[表情]"
		} else if _, ok := elem.(*message.MarketFaceElement); ok {
			tmpText += "[表情]"
		} else if _, ok := elem.(*message.GroupImageElement); ok {
			tmpText += "[图片]"
		} else if _, ok := elem.(*message.FriendImageElement); ok {
			tmpText += "[图片]"
		} else if _, ok := elem.(*message.ReplyElement); ok {
			tmpText += "[引用]"
		} else if _, ok := elem.(*message.VoiceElement); ok {
			tmpText += "[语音]"
		}
	}
	if !mode {
		name := Msg.(*message.GroupMessage).Sender.CardName
		if name == "" {
			name = Msg.(*message.GroupMessage).Sender.Nickname
		}
		logger.Infof("收到群 %s(%d) 内 %s(%d) 的消息: %s (%d)", Msg.(*message.GroupMessage).GroupName, Msg.(*message.GroupMessage).GroupCode, name, Msg.(*message.GroupMessage).Sender.Uin, tmpText, Msg.(*message.GroupMessage).Id)
		//logger.Debugf("%+v", Msg.(*message.GroupMessage))
	} else {
		logger.Infof("收到 %s(%d) 的私聊消息: %s (%d)", Msg.(*message.PrivateMessage).Sender.Nickname, Msg.(*message.PrivateMessage).Sender.Uin, tmpText, Msg.(*message.PrivateMessage).Id)
		//logger.Debugf("%+v", Msg.(*message.PrivateMessage))
	}
}

func (c *QQClient) waitDataAndDispatch(g *message.GroupMessage) {
	if c.GroupList != nil {
		c.SetMsgGroupNames(g)
	}
	if c.FriendList != nil {
		c.SetFriend(g)
	}
	// 使用 dispatch 方法
	c.GroupMessageEvent.dispatch(c, g)
	c.OutputReceivingMessage(g)
}

func (c *QQClient) SetFriend(g *message.GroupMessage) {
	g.Sender.IsFriend = c.FindFriend(g.Sender.Uin) != nil
}

func (c *QQClient) SetMsgGroupNames(g *message.GroupMessage) {
	group := c.FindGroupByUin(g.GroupCode)
	if group != nil {
		g.GroupName = group.Name
	} else {
		logger.Warnf("Not found group name: %d", g.GroupCode)
	}
}

func (c *QQClient) SyncGroupMembers(groupID DynamicInt64) {
	//time.Sleep(time.Second * 3)
	// if flash {
	// 	//logger.Info("start reload groups list")
	// 	err := c.ReloadGroupList()
	// 	logger.Infof("load %d groups", len(c.GroupList))
	// 	if err != nil {
	// 		logger.WithError(err).Error("unable to load groups list")
	// 	}
	// 	err = c.ReloadGroupMembers()
	// 	if err != nil {
	// 		logger.WithError(err).Error("unable to load group members list")
	// 	} else {
	// 		logger.Info("load members done.")
	// 	}
	// } else {
	group := c.FindGroupByUin(groupID.ToInt64())
	// sort.Slice(c.GroupList, func(i2, j int) bool {
	// 	return c.GroupList[i2].Uin < c.GroupList[j].Uin
	// })
	// i := sort.Search(len(c.GroupList), func(i int) bool {
	// 	return c.GroupList[i].Uin >= int64(groupID)
	// })
	// if i >= len(c.GroupList) || c.GroupList[i].Uin != int64(groupID) {
	// 	return
	// }
	//c.GroupList[i].Members, err = c.getGroupMembers(c.GroupList[i], intern)
	if group != nil {
		var err error
		group.Members, err = c.GetGroupMembers(group)
		if err != nil {
			logger.Errorf("Failed to update group members: %v", err)
		}
	} else {
		logger.Warnf("Not found group: %d", groupID.ToInt64())
	}
	// }
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
		c.ws = ws
		// allow 10 requests per second
		c.limiterMessageSend = NewRateLimiter(time.Second, 5, func() Window {
			return NewLocalWindow()
		})

		//初始化变量
		c.responseChans = make(map[string]chan *ResponseGroupsData)
		c.responseGroupChans = make(map[string]chan *ResponseGroupData)
		c.responseFriends = make(map[string]chan *ResponseFriendList)
		c.responseMembers = make(map[string]chan *ResponseGroupMemberData)
		c.responseMessage = make(map[string]chan *ResponseSendMessage)
		c.responseSetLeave = make(map[string]chan *ResponseSetGroupLeave)
		c.responseStrangerInfo = make(map[string]chan *ResponseGetStrangerInfo)

		// 打印客户端的 headers
		for name, values := range r.Header {
			for _, value := range values {
				logger.WithField(name, value).Debug()
				//log.Printf("%s: %s", name, value)
			}
		}
		go c.RefreshList()
		go c.SendLimit()
		go c.processMessage()
		c.handleConnection(ws)

	})

	wsAddr := config.GlobalConfig.GetString("ws-server")
	if wsAddr == "" {
		wsAddr = "0.0.0.0:15630"
	}
	logger.WithField("force", true).Printf("WebSocket server started on ws://%s/ws", wsAddr)
	logger.Fatal(http.ListenAndServe(wsAddr, nil))
	//log.Println("WebSocket server started on ws://0.0.0.0:15630")
	//log.Fatal(http.ListenAndServe("0.0.0.0:15630", nil))
}

// RefreshList 刷新联系人
func (c *QQClient) RefreshList() {
	reloadDelay := config.GlobalConfig.GetBool("reloadDelay.enable")
	if reloadDelay {
		//logger.Info("enabled reload delay")
		var Delay time.Duration
		Delay = config.GlobalConfig.GetDuration("reloadDelay.time")
		logger.Infof("延迟加载时间: %ss", strconv.FormatFloat(Delay.Seconds(), 'f', 0, 64))
		time.Sleep(Delay)
	}
	//logger.Info("start reload friends list")
	err := c.ReloadFriendList()
	if err != nil {
		logger.WithError(err).Error("unable to load friends list")
	}
	logger.Infof("已加载 %d 个好友", len(c.FriendList))
	//logger.Info("start reload groups list")
	err = c.ReloadGroupList()
	if err != nil {
		logger.WithError(err).Error("unable to load groups list")
	}
	logger.Infof("已加载 %d 个群组", len(c.GroupList))
	//logger.Info("start reload group members list")
	err = c.ReloadGroupMembers()
	if err != nil {
		logger.WithError(err).Error("unable to load group members list")
	} else {
		logger.Info("加载 群员信息 完成")
	}
}

func (c *QQClient) ReloadGroupMembers() error {
	var err error
	for _, group := range c.GroupList {
		group.Members = nil
		group.Members, err = c.GetGroupMembers(group)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *QQClient) SendGroupMessage(groupCode int64, m *message.SendingMessage, newstr string) SendResp {
	resultChan := make(chan SendResp)
	sendMsg := SendMsg{
		GroupCode:  groupCode,
		Message:    m,
		NewStr:     newstr,
		ResultChan: resultChan,
	}
	sendMessageQueue <- sendMsg
	return <-resultChan
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
	c.FriendList = nil
	c.FriendList = rsp.List
	return nil
}

func (c *QQClient) GetFriendList() (*FriendListResponse, error) {

	echo := generateEcho("get_friend_list")
	// 构建请求
	req := map[string]interface{}{
		"action": "get_friend_list",
		"params": map[string]int64{},
		"echo":   echo,
	}
	// 创建响应通道并添加到映射中
	respChan := make(chan *ResponseFriendList)
	// 初始化 c.responseChans 映射
	//c.responseFriends = make(map[string]chan *ResponseFriendList)
	c.responseFriends[echo] = respChan

	// 发送请求
	data, _ := json.Marshal(req)
	c.sendToWebSocketClient(c.ws, data)

	// 等待响应或超时
	select {
	case resp := <-respChan:
		delete(c.responseFriends, echo)
		friends := make([]*FriendInfo, len(resp.Data))
		c.debug("GetFriendList: %v", resp.Data)
		for i, friend := range resp.Data {
			friends[i] = &FriendInfo{
				Uin:      friend.UserID,
				Nickname: friend.NickName,
				Remark:   friend.Remark,
				FaceId:   0,
			}
		}
		return &FriendListResponse{TotalCount: int32(len(friends)), List: friends}, nil

	case <-time.After(10 * time.Second):
		delete(c.responseFriends, echo)
		return nil, errors.New("GetFriendList: timeout waiting for response.")
	}
}

// GetFriendList
// 当使用普通QQ时: 请求好友列表
// 当使用企点QQ时: 请求外部联系人列表
// func (c *QQClient) GetFriendList() (*FriendListResponse, error) {
// 	if c.version().Protocol == auth.QiDian {
// 		rsp, err := c.getQiDianAddressDetailList()
// 		if err != nil {
// 			return nil, err
// 		}
// 		return &FriendListResponse{TotalCount: int32(len(rsp)), List: rsp}, nil
// 	}
// 	curFriendCount := 0
// 	r := &FriendListResponse{}
// 	for {
// 		rsp, err := c.sendAndWait(c.buildFriendGroupListRequestPacket(int16(curFriendCount), 150, 0, 0))
// 		if err != nil {
// 			return nil, err
// 		}
// 		list := rsp.(*FriendListResponse)
// 		r.TotalCount = list.TotalCount
// 		r.List = append(r.List, list.List...)
// 		curFriendCount += len(list.List)
// 		if int32(len(r.List)) >= r.TotalCount {
// 			break
// 		}
// 	}
// 	return r, nil
// }

func (c *QQClient) GetGroupInfo(groupCode int64) (*GroupInfo, error) {
	echo := generateEcho("get_group_info")
	// 构建请求
	req := map[string]interface{}{
		"action": "get_group_info",
		"params": map[string]int64{
			"group_id": groupCode,
		},
		"echo": echo,
	}
	// 创建响应通道并添加到映射中
	respChan := make(chan *ResponseGroupData)
	c.responseGroupChans[echo] = respChan

	// 发送请求
	data, _ := json.Marshal(req)
	c.sendToWebSocketClient(c.ws, data)

	// 等待响应或超时
	select {
	case resp := <-respChan:
		if resp.Retcode != 0 {
			return nil, errors.New(resp.Message)
		}
		delete(c.responseGroupChans, echo)
		c.debug("GetGroupInfo: %v", resp.Data)
		group := &GroupInfo{
			Uin:             resp.Data.GroupID,
			Code:            resp.Data.GroupID,
			Name:            resp.Data.GroupName,
			GroupCreateTime: uint32(resp.Data.GroupCreateTime),
			GroupLevel:      uint32(resp.Data.GroupLevel),
			MemberCount:     uint16(resp.Data.MemberCount),
			MaxMemberCount:  uint16(resp.Data.MaxMemberCount),
		}
		return group, nil
	case <-time.After(10 * time.Second):
		delete(c.responseGroupChans, echo)
		return nil, errors.New("GetGroupInfo: timeout waiting for response.")
	}
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
	c.GroupList = nil
	c.GroupList = list
	return nil
}

func generateEcho(action string) string {
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s:%d", action, timestamp)
}

func (c *QQClient) GetGroupList() ([]*GroupInfo, error) {
	echo := generateEcho("get_group_list")
	// 构建请求
	req := map[string]interface{}{
		"action": "get_group_list",
		"params": map[string]string{},
		"echo":   echo,
	}
	// 创建响应通道并添加到映射中
	respChan := make(chan *ResponseGroupsData)
	// 初始化 c.responseChans 映射
	// c.responseChans = make(map[string]chan *ResponseGroupsData)
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

		// 将ResponseGroupsData转换为GroupInfo列表
		delete(c.responseChans, echo)
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
		delete(c.responseChans, echo)
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
		"params": map[string]int64{
			"group_id": group.Uin,
		},
		"echo": echo,
	}
	// 创建响应通道并添加到映射中
	respChan := make(chan *ResponseGroupMemberData)
	// 初始化 c.responseChans 映射
	// c.responseMembers = make(map[string]chan *ResponseGroupMemberData)
	c.responseMembers[echo] = respChan

	// 发送请求
	data, _ := json.Marshal(req)
	c.sendToWebSocketClient(c.ws, data)

	// 等待响应或超时
	select {
	case resp := <-respChan:
		delete(c.responseMembers, echo)
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
		delete(c.responseMembers, echo)
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
	subType := ""
	var Req interface{}
	switch req := i.(type) {
	case *UserJoinGroupRequest:
		// _, pkt := c.buildSystemMsgGroupActionPacket(req.RequestId, req.RequesterUin, req.GroupCode, func() int32 {
		// 	if req.Suspicious {
		// 		return 2
		// 	} else {
		// 		return 1
		// 	}
		// }(), false, accept, block, reason)
		// _ = c.sendPacket(pkt)
		subType = "add"
		Req = req
	case *GroupInvitedRequest:
		// _, pkt := c.buildSystemMsgGroupActionPacket(req.RequestId, req.InvitorUin, req.GroupCode, 1, true, accept, block, reason)
		// _ = c.sendPacket(pkt)
		subType = "invite"
		Req = req
	}
	echo := generateEcho("set_group_add_request")
	// 构建请求
	newReq := map[string]interface{}{
		"action": "set_group_add_request",
		"params": map[string]interface{}{
			"sub_type": subType,
			"approve":  accept,
			"reason":   reason,
		},
		"echo": echo,
	}
	if _, ok := Req.(*UserJoinGroupRequest); ok {
		newReq["params"].(map[string]interface{})["flag"] = Req.(*UserJoinGroupRequest).Flag
	} else if _, ok := Req.(*GroupInvitedRequest); ok {
		newReq["params"].(map[string]interface{})["flag"] = Req.(*GroupInvitedRequest).Flag
	}
	logger.Debugf("加群回复构建完成：%s", newReq)
	// 发送请求
	data, _ := json.Marshal(newReq)
	c.sendToWebSocketClient(c.ws, data)
}

func (c *QQClient) SolveFriendRequest(req *NewFriendRequest, accept bool) {
	echo := generateEcho("set_friend_add_request")
	// 构建请求
	newReq := map[string]interface{}{
		"action": "set_friend_add_request",
		"params": map[string]interface{}{
			"flag":    req.Flag,
			"approve": accept,
		},
		"echo": echo,
	}
	// 发送请求
	data, _ := json.Marshal(newReq)
	c.sendToWebSocketClient(c.ws, data)
}

// func (c *QQClient) SolveFriendRequest(req *NewFriendRequest, accept bool) {
// 	_, pkt := c.buildSystemMsgFriendActionPacket(req.RequestId, req.RequesterUin, accept)
// 	_ = c.sendPacket(pkt)
// }

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

func (c *QQClient) quitGroup(group *GroupInfo) {
	//_, _ = c.sendAndWait(c.buildQuitGroupPacket(groupCode))
	//group = c.FindGroup(group.Code)
	echo := generateEcho("set_group_leave")
	// 构建请求
	req := map[string]interface{}{
		"action": "set_group_leave",
		"params": map[string]interface{}{
			"group_id":   group.Code,
			"is_dismiss": false,
			// "group_id":   strconv.FormatInt(group.Code, 10),
			// "is_dismiss": strconv.FormatBool(false),
		},
		"echo": echo,
	}
	// 创建响应通道并添加到映射中
	respChan := make(chan *ResponseSetGroupLeave)
	// 初始化 c.responseChans 映射
	//c.responseSetLeave = make(map[string]chan *ResponseSetGroupLeave)
	c.responseSetLeave[echo] = respChan
	// 发送请求
	data, _ := json.Marshal(req)
	c.sendToWebSocketClient(c.ws, data)
	// 等待响应或超时
	select {
	case resp := <-respChan:
		delete(c.responseSetLeave, echo)
		if resp.Retcode == 0 {
			sort.Slice(c.GroupList, func(i, j int) bool {
				return c.GroupList[i].Code < c.GroupList[j].Code
			})
			i := sort.Search(len(c.GroupList), func(i int) bool {
				return c.GroupList[i].Code >= group.Code
			})
			if i < len(c.GroupList) && c.GroupList[i].Code == group.Code {
				c.GroupList = append(c.GroupList[:i], c.GroupList[i+1:]...)
			}
			logger.Infof("退出群组成功")
		} else {
			logger.Errorf("退出群组失败: %v", resp.Message)
		}
	case <-time.After(10 * time.Second):
		delete(c.responseSetLeave, echo)
		logger.Errorf("退出群组超时")
	}
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

func (c *QQClient) GetStrangerInfo(uid int64) (ResponseGetStrangerInfo, error) {
	echo := generateEcho("get_stranger_info")
	// 构建请求
	req := map[string]interface{}{
		"action": "get_stranger_info",
		"params": map[string]interface{}{
			"user_id":  uid,
			"no_cache": false,
		},
		"echo": echo,
	}
	// 创建响应通道并添加到映射中
	respChan := make(chan *ResponseGetStrangerInfo)
	c.responseStrangerInfo[echo] = respChan
	// 发送请求
	data, _ := json.Marshal(req)
	c.sendToWebSocketClient(c.ws, data)
	// 等待响应或超时
	select {
	case resp := <-respChan:
		delete(c.responseStrangerInfo, echo)
		if resp.Retcode == 0 {
			logger.Debug("GetStrangerInfo Success")
			return *resp, nil
		} else {
			return ResponseGetStrangerInfo{}, errors.New(resp.Message)
		}
	case <-time.After(10 * time.Second):
		delete(c.responseStrangerInfo, echo)
		return ResponseGetStrangerInfo{}, errors.New("get stranger info timeout")
	}
}
