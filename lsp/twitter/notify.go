package twitter

import (
	"fmt"
	"github.com/Sora233/DDBOT/lsp/mmsg"
	"github.com/Sora233/DDBOT/proxy_pool"
	"github.com/Sora233/DDBOT/requests"
	localutils "github.com/Sora233/DDBOT/utils"
	"github.com/sirupsen/logrus"
	"math"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

type NewNotify struct {
	groupCode int64
	*NewsInfo
}

func (n *NewNotify) GetGroupCode() int64 {
	return n.groupCode
}

func (n *NewNotify) ToMessage() *mmsg.MSG {
	t := n.GetTweetContent(n.Tweet.GetId())
	if t.XTypename == "TweetTombstone" {
		logger.WithField("TweetId", n.Tweet.GetId()).
			Warnf("tweet to tombstone: %s", t.Tombstone.Text.Text)
		return &mmsg.MSG{}
	}
	var reTweetNotify *NewNotify
	if n.Tweet.Type == RETWEET {
		reTweetNotify = n
	}
	message := t.GetTweetMessage(reTweetNotify)
	return message
}

func (n *NewNotify) Logger() *logrus.Entry {
	return n.NewsInfo.Logger().WithFields(localutils.GroupLogFields(n.groupCode))
}

// Base36Chars 定义了用于 36 进制表示的字符集。
const Base36Chars = "0123456789abcdefghijklmnopqrstuvwxyz"

// floatToBase36 将 float64 转换为其 36 进制字符串表示形式，
// 尝试模拟 JavaScript 的 Number.prototype.toString(36)。
// 注意：由于 JS 引擎内部实现的差异，精确的复制可能有所不同。
// precision 参数控制小数部分转换的最大位数。
func floatToBase36(val float64, precision int) string {
	// 单独处理零值
	if val == 0 {
		return "0"
	}

	// 处理 NaN 和 Inf
	// JS 的 toString(36) 会返回 "NaN" 或 "Infinity"
	// 后续的替换操作将应用于这些字符串。
	if math.IsNaN(val) {
		return "NaN"
	}
	if math.IsInf(val, 1) {
		return "Infinity"
	}
	if math.IsInf(val, -1) {
		// JS 中 Number 类型没有负无穷的概念，但 Go float64 有
		// 为完整起见，进行处理，尽管原始 JS 代码可能不会产生负无穷
		return "-Infinity"
	}

	// 处理负数
	sign := ""
	if val < 0 {
		sign = "-"
		val = -val // 取绝对值进行转换
	}

	// 分离整数和小数部分
	intPart := int64(val) // 直接截断取整
	fracPart := val - float64(intPart)

	// 转换整数部分
	intStr := strconv.FormatInt(intPart, 36)

	// 转换小数部分
	var fracStrBuilder strings.Builder
	// 使用一个小的 epsilon 来比较浮点数，避免精度问题
	const epsilon = 1e-9 // 容差值
	if fracPart > epsilon {
		for i := 0; i < precision && fracPart > epsilon; i++ {
			// 将小数部分乘以 36
			fracPart *= 36
			// 取整数部分作为当前位的数字 (0-35)
			digit := int(fracPart)

			// 健壮性检查：确保 digit 在有效范围内
			if digit < 0 || digit >= 36 {
				// 这通常表示浮点精度问题或计算错误
				// 在这里停止处理小数部分是安全的
				fmt.Printf("警告：在转换小数部分的 36 进制时遇到无效数字 %d。\n", digit)
				break
			}
			// 将数字转换为对应的 36 进制字符并追加
			fracStrBuilder.WriteByte(Base36Chars[digit])
			// 减去整数部分，继续处理剩余的小数
			fracPart -= float64(digit)
		}
	}

	// 如果存在小数部分转换结果，则用 "." 连接整数和小数部分
	if fracStrBuilder.Len() > 0 {
		return sign + intStr + "." + fracStrBuilder.String()
	}
	// 否则只返回整数部分的转换结果
	return sign + intStr
}

// GetToken 将 JavaScript 的 getToken 函数移植到 Go。
// 它接收一个 ID 字符串，执行计算，转换为 36 进制，并移除 '0' 和 '.'。
func (n *NewNotify) getToken(id string) (string, error) {
	// 1. 将 id 字符串转换为 float64
	f, err := strconv.ParseFloat(id, 64)
	if err != nil {
		// 返回中文错误信息
		return "", fmt.Errorf("无法将 ID '%s' 解析为数字: %w", id, err)
	}

	// 2. 执行计算: (Number(id) / 1e15) * Math.PI
	val := (f / 1e15) * math.Pi

	// 3. 将结果转换为 36 进制字符串
	// 为小数部分使用一个合理的精度（例如 15 位），与 float64 的精度限制类似
	base36Str := floatToBase36(val, 15)

	// 4. 移除所有 '.' 字符，然后移除所有 '0' 字符
	// 这与 JS 正则表达式 /(0+|\.)/g 的行为相匹配
	// (先移除点，再移除零，效果等同于移除点和所有零)
	result := strings.ReplaceAll(base36Str, ".", "")
	result = strings.ReplaceAll(result, "0", "")

	// 处理 NaN/Infinity 转换后的结果
	// JS: "NaN".replace(/(0+|\.)/g, '') -> "NaN"
	// JS: "Infinity".replace(/(0+|\.)/g, '') -> "Infinity"
	// Go 的替换逻辑对这些特定字符串也能得到相同结果。

	return result, nil
}
func (n *NewNotify) GetTweetContent(tid string) *TweetMessage {
	token, err := n.getToken(tid)
	if err != nil {
		n.Logger().WithField("GetToken:", tid).
			Errorf("get token failed: %v", err)
	}
	getTweetUrl := fmt.Sprintf(TweetAPI, tid, token)
	opts := []requests.Option{
		requests.ProxyOption(proxy_pool.PreferOversea),
		requests.RetryOption(3),
		requests.TimeoutOption(time.Second * 10),
	}
	resp := new(TweetMessage)
	err = requests.Get(getTweetUrl, nil, resp, opts...)
	if err != nil {
		n.Logger().WithField("GeTweet:", tid).
			Errorf("get tweet content failed: %v", err)
		return &TweetMessage{}
	}
	return resp
}

func (t *TweetMessage) GetTweetMessage(reTweetNotify *NewNotify) *mmsg.MSG {
	defer func() {
		if err := recover(); err != nil {
			logger.WithField("stack", string(debug.Stack())).
				WithField("tweet", t).
				Errorf("concern notify recoverd %v", err)
		}
	}()
	// 构造消息
	message := mmsg.NewMSG()
	if t == nil {
		return message
	}
	location, _ := time.LoadLocation("Asia/Shanghai")
	var CreatedAt time.Time
	if reTweetNotify != nil {
		CreatedAt = reTweetNotify.LatestNewsTs
		message.Textf(fmt.Sprintf("X-%s转发了%s的%s：\n",
			reTweetNotify.Tweet.Author.Name, t.User.Name, t.XTypename))
	} else {
		CreatedAt, _ = time.Parse(time.RFC3339, t.CreatedAt)
		message.Textf(fmt.Sprintf("X-%s发布了新%s：\n", t.User.Name, t.XTypename))
	}

	message.Text(CSTTime(CreatedAt).In(location).Format(time.DateTime) + "\n")
	// 提取URL
	urlRegex := regexp.MustCompile(`\s+(https?://\S+)$`)
	matches := urlRegex.FindStringSubmatch(t.Text)
	// 删除推文中的URL
	var extractedURL string
	if len(matches) > 1 {
		t.Text = strings.TrimSuffix(t.Text, matches[1])
		extractedURL = matches[1]
	}
	// msg加入推文
	message.Text(t.Text + "\n")
	// msg加入图片
	for _, p := range t.Photos {
		message.Append(
			mmsg.NewImageByUrl(p.Url,
				requests.ProxyOption(proxy_pool.PreferOversea),
				requests.TimeoutOption(time.Second*10),
				requests.RetryOption(3)))
	}
	// msg加入url
	if extractedURL != "" {
		message.Text(extractedURL + "\n")
	}
	// msg加入视频
	for _, v := range t.MediaDetails {
		message.Cut()
		message.Append(
			mmsg.NewVideoByUrl(v.VideoInfo.Variants[1].Url,
				requests.ProxyOption(proxy_pool.PreferOversea),
				requests.TimeoutOption(time.Second*10),
				requests.RetryOption(3)))
	}
	return message
}
