// 别忘记改package name
package twitter

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Sora233/DDBOT/lsp/buntdb"
	"github.com/Sora233/DDBOT/lsp/concern"
	"github.com/Sora233/DDBOT/lsp/concern_type"
	"github.com/Sora233/DDBOT/lsp/mmsg"
	"github.com/Sora233/DDBOT/proxy_pool"
	"github.com/Sora233/DDBOT/requests"
	"github.com/Sora233/MiraiGo-Template/utils"
	"github.com/mmcdole/gofeed"
)

const (
	// 这个名字是日志中的名字，如果不知道取什么名字，可以和Site一样
	ConcernName = "twitter-concern"

	// 插件支持的网站名
	Site = "twitter"
	// 这个插件支持的订阅类型可以像这样自定义，然后在 Types 中返回
	Tweets concern_type.Type = "news"
	// 当像这样定义的时候，支持 /watch -s mysite -t type1 id
	// 当实现的时候，请修改上面的定义
	// API Base URL
	BaseURL = "https://nitter.poast.org/%s/rss"
	//alt1BaseURL = "https://nitter.privacydev.net/%s/rss"
	TweetAPI = "https://cdn.syndication.twimg.com/tweet-result?id=%s&token=%s"
	//UserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36 Edg/134.0.0.0"
)

var logger = utils.GetModuleLogger(ConcernName)

type twitterStateManager struct {
	*concern.StateManager
}

// GetGroupConcernConfig 重写 concern.StateManager 的GetGroupConcernConfig方法，让我们自己定义的 GroupConcernConfig 生效
func (t *twitterStateManager) GetGroupConcernConfig(groupCode int64, id interface{}) concern.IConfig {
	return NewGroupConcernConfig(t.StateManager.GetGroupConcernConfig(groupCode, id))
}

type twitterConcern struct {
	*twitterStateManager
	*extraKey
	parser *gofeed.Parser
}

func (t *twitterConcern) Site() string {
	return Site
}

func (t *twitterConcern) Types() []concern_type.Type {
	return []concern_type.Type{Tweets}
}

func (t *twitterConcern) ParseId(s string) (interface{}, error) {
	// 在这里解析id
	// 此处返回的id类型，即是其他地方id interface{}的类型
	// 其他所有地方的id都由此函数生成
	// 推荐在string 或者 int64类型中选择其一
	// 如果订阅源有uid等数字唯一标识，请选择int64，如 bilibili
	// 如果订阅源有数字并且有字符，请选择string， 如 douyu
	if strings.HasPrefix(s, "@") {
		return strings.TrimPrefix(s, "@"), nil
	}
	return s, nil
}

func buildProfileURL(screenName string) string {
	return strings.ReplaceAll(BaseURL, "%s", screenName)
}

func CSTTime(t time.Time) time.Time {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	return t.In(loc)
}

func (t *twitterConcern) FindUserInfo(id string, refresh bool) (*UserInfo, error) {
	var info *UserInfo
	if refresh {
		Url := buildProfileURL(id)
		opts := []requests.Option{
			requests.ProxyOption(proxy_pool.PreferOversea),
			requests.TimeoutOption(time.Second * 10),
			requests.AddUAOption(),
			requests.RetryOption(3),
		}
		var resp bytes.Buffer
		if err := requests.Get(Url, nil, &resp, opts...); err != nil {
			return nil, fmt.Errorf("FindUserInfo error: %v", err)
		}
		data := io.Reader(&resp)
		feed, err := t.parser.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("parse UserInfo error: %v", err)
		}
		info = &UserInfo{
			Id:              id,
			Name:            feed.Title,
			ProfileImageUrl: feed.Image.URL,
		}
		info.Name = GetShortName(feed)
		err = t.AddUserInfo(info)
		if err != nil {
			return nil, fmt.Errorf("Error adding user info: %v", err)
		}
	}
	return t.GetUserInfo(id)
}

func (t *twitterConcern) FindOrLoadUserInfo(id string) (*UserInfo, error) {
	info, _ := t.FindUserInfo(id, false)
	if info == nil {
		return t.FindUserInfo(id, true)
	}
	return info, nil
}

func (t *twitterConcern) GetUserInfo(id string) (*UserInfo, error) {
	var userInfo *UserInfo
	err := t.GetJson(t.UserInfoKey(id), &userInfo)
	if err != nil {
		return nil, err
	}
	return userInfo, nil
}

func (t *twitterConcern) AddUserInfo(info *UserInfo) error {
	if info == nil {
		return errors.New("<nil userInfo>")
	}
	return t.SetJson(t.UserInfoKey(info.Id), info)
}

func (t *twitterConcern) AddNewsInfo(info *NewsInfo) error {
	if info == nil {
		return errors.New("<nil NewsInfo>")
	}
	return t.RWCover(func() error {
		var err error
		err = t.SetJson(t.UserInfoKey(info.UserInfo.Id), info.UserInfo)
		if err != nil {
			return err
		}
		return t.SetJson(t.NewsInfoKey(info.UserInfo.Id), info)
	})
}

func (t *twitterConcern) Add(ctx mmsg.IMsgCtx, groupCode int64, id interface{}, ctype concern_type.Type) (concern.IdentityInfo, error) {
	// 这里是添加订阅的函数
	// 可以使 c.StateManager.AddGroupConcern(groupCode, id, ctype) 来添加这个订阅
	// 通常在添加订阅前还需要通过id访问网站上的个人信息页面，来确定id是否存在，是否可以正常订阅
	info, err := t.FindOrLoadUserInfo(id.(string))
	if err != nil {
		return nil, fmt.Errorf("查询用户信息失败 %v - %v", id, err)
	}
	err = t.AddNewsInfo(&NewsInfo{
		UserInfo:     info,
		LatestNewsTs: time.Now().UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("添加订阅失败 - 内部错误 - %v", err)
	}
	_, err = t.GetStateManager().AddGroupConcern(groupCode, id, ctype)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (t *twitterConcern) removeNewsInfo(id string) error {
	_, err := t.Delete(t.NewsInfoKey(id), buntdb.IgnoreNotFoundOpt())
	return err
}

func (t *twitterConcern) removeUserInfo(id string) error {
	_, err := t.Delete(t.UserInfoKey(id), buntdb.IgnoreNotFoundOpt())
	return err
}

func (t *twitterConcern) Remove(ctx mmsg.IMsgCtx, groupCode int64, id interface{}, ctype concern_type.Type) (concern.IdentityInfo, error) {
	// 大部分时候简单的删除即可
	// 如果还有更复杂的逻辑可以自由实现
	identity, _ := t.Get(id)
	_, err := t.GetStateManager().RemoveGroupConcern(groupCode, id.(string), ctype)
	if err != nil {
		return nil, err
	}

	if err = t.removeNewsInfo(id.(string)); err != nil {
		if err != errors.New("not found") {
			logger.WithError(err).Errorf("removeNewsInfo error")
		} else {
			err = nil
		}
	}

	if err = t.removeUserInfo(id.(string)); err != nil {
		if err != errors.New("not found") {
			logger.WithError(err).Errorf("removeUserInfo error")
		} else {
			err = nil
		}
	}

	if identity == nil {
		identity = concern.NewIdentity(id, "unknown")
	}
	return identity, err
}

func (t *twitterConcern) Get(id interface{}) (concern.IdentityInfo, error) {
	// 查看一个订阅的信息
	// 通常是查看数据库中是否有id的信息，如果没有可以去网页上获取
	usrInfo, err := t.GetUserInfo(id.(string))
	if err != nil {
		return nil, errors.New("GetUserInfo error")
	}
	return concern.NewIdentity(usrInfo.Id, usrInfo.Name), nil
}

func (t *twitterConcern) notifyGenerator() concern.NotifyGeneratorFunc {
	return func(groupCode int64, event concern.Event) []concern.Notify {
		switch event.(type) {
		case *NewsInfo:
			return []concern.Notify{&NewNotify{groupCode, event.(*NewsInfo)}}
		default:
			logger.Errorf("unknown EventType %+v", event)
			return nil
		}
	}
}

func (t *twitterConcern) fresh() concern.FreshFunc {
	return t.EmitQueueFresher(func(ctype concern_type.Type, id interface{}) ([]concern.Event, error) {
		var result []concern.Event
		userId := id.(string)
		if ctype.ContainAll(Tweets) {
			userInfo, err := t.FindOrLoadUserInfo(userId)
			newsInfo := &NewsInfo{UserInfo: userInfo}
			if err != nil {
				logger.WithError(err).Error("FindOrLoadUserInfo error")
			}
			newTweets, err := t.GetTweets(userId)
			if err != nil {
				return nil, err
			}
			oldNewsInfo, err := t.GetNewsInfo(userId)
			if err != nil {
				return nil, err
			}
			newsInfo.LatestNewsTs = time.Now().UTC()
			if len(newTweets) > 0 && newTweets[0].GetId() != "" {
				//newsInfo.LatestNewsTs = t.GetLatestNewsTs(newTweets)
				newsInfo.LatestTweetId = newTweets[0].GetId()
				if oldNewsInfo == nil || (oldNewsInfo != nil && newsInfo.LatestTweetId != oldNewsInfo.LatestTweetId) {
					if oldNewsInfo.LatestTweetId == "" {
						oldNewsInfo = &NewsInfo{
							UserInfo:      userInfo,
							LatestNewsTs:  newsInfo.LatestNewsTs,
							LatestTweetId: newsInfo.LatestTweetId,
						}
					}
					// 获取超过最后推送时间的tweet
					//NewTweets := t.GetNewTweetsFromTime(oldNewsInfo.LatestNewsTs, newTweets)
					NewTweets := t.GetNewTweetsFromTweetId(oldNewsInfo.LatestTweetId, newTweets)
					if len(NewTweets) > 0 {
						t.reverseTweets(NewTweets)
						// 将新的tweet添加到result中
						for _, tweet := range NewTweets {
							res := &NewsInfo{
								UserInfo:     userInfo,
								Tweet:        tweet,
								LatestNewsTs: tweet.Published,
							}
							if tweet.Type == RETWEET {
								res.LatestNewsTs = newsInfo.LatestNewsTs
							}
							result = append(result, res)
						}
					}
				}
			}
			err = t.AddNewsInfo(newsInfo)
			if err != nil {
				logger.Errorf("AddNewsInfo error %v", err)
				return nil, err
			}
		}
		return result, nil
	})
}

func (t *twitterConcern) GetTweets(id string) ([]*TweetItem, error) {
	Url := buildProfileURL(id)
	opts := []requests.Option{
		requests.ProxyOption(proxy_pool.PreferOversea),
		requests.TimeoutOption(time.Second * 10),
		requests.AddUAOption(),
		requests.RetryOption(3),
	}
	var resp bytes.Buffer
	if err := requests.Get(Url, nil, &resp, opts...); err != nil {
		return nil, fmt.Errorf("GetTweets error: %v", err)
	}
	data := io.Reader(&resp)
	feed, err := t.parser.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("GetTweets error: %v", err)
	}
	var result []*TweetItem
	for _, item := range feed.Items {
		tweetType := TWEET
		if strings.HasPrefix(item.Title, "RT by @") {
			tweetType = RETWEET
		}
		result = append(result, &TweetItem{
			Type:        tweetType,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Published:   *item.PublishedParsed,
			Author: &UserInfo{
				Id:              id,
				Name:            GetShortName(feed),
				ProfileImageUrl: feed.Image.URL,
			},
		})
	}
	// 时间排序
	//t.sortTweetsByPublished(result)
	return result, nil
}

//func (t *twitterConcern) sortTweetsByPublished(tweets []*TweetItem) {
//	sort.SliceStable(tweets, func(i, j int) bool {
//		return tweets[i].Published.Before(tweets[j].Published)
//	})
//}

func (t *twitterConcern) reverseTweets(s []*TweetItem) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func (t *twitterConcern) GetNewTweetsFromTweetId(oldId string, tweets []*TweetItem) []*TweetItem {
	if index := findTweetIndex(tweets, oldId); index >= 0 {
		return tweets[:index]
	}
	oldTime, err := ParseSnowflakeTimestamp(oldId)
	if err != nil {
		logger.WithError(err).Errorf("ParseSnowflakeTimestamp error")
	}
	if tweets[0].Published.Before(oldTime) {
		return nil
	}
	return tweets
}

func findTweetIndex(tweets []*TweetItem, targetID string) int {
	for i, tweet := range tweets {
		if tweet.GetId() == targetID {
			return i
		}
	}
	return -1
}

//func (t *twitterConcern) GetNewTweetsFromTime(oldTime time.Time, item []*TweetItem) []*TweetItem {
//	var result []*TweetItem
//	for _, tweet := range item {
//		if tweet.Published.After(oldTime) {
//			result = append(result, tweet)
//		}
//	}
//	return result
//}

func (t *twitterConcern) GetLatestNewsTs(tweets []*TweetItem) time.Time {
	return tweets[len(tweets)-1].Published
}

func (t *twitterConcern) GetNewsInfo(id string) (*NewsInfo, error) {
	var newsInfo *NewsInfo
	err := t.GetJson(t.NewsInfoKey(id), &newsInfo)
	if err != nil {
		return nil, err
	}
	return newsInfo, nil
}

func (t *twitterConcern) Start() error {
	// 如果需要启用轮询器，可以使用下面的方法
	t.UseEmitQueue()
	// 下面两个函数是订阅的关键，需要实现，请阅读文档
	t.StateManager.UseFreshFunc(t.fresh())
	t.StateManager.UseNotifyGeneratorFunc(t.notifyGenerator())
	return t.StateManager.Start()
}

func (t *twitterConcern) Stop() {
	logger.Tracef("正在停止%v concern", Site)
	logger.Tracef("正在停止%v StateManager", Site)
	t.StateManager.Stop()
	logger.Tracef("%v StateManager已停止", Site)
	logger.Tracef("%v concern已停止", Site)
}

func (t *twitterConcern) GetStateManager() concern.IStateManager {
	return t.StateManager
}

func newConcern(notifyChan chan<- concern.Notify) *twitterConcern {
	// 默认是string格式的id
	sm := &twitterStateManager{concern.NewStateManagerWithStringID(Site, notifyChan)}
	// 如果要使用int64格式的id，可以用下面的
	//c.StateManager = concern.NewStateManagerWithInt64ID(Site, notifyChan)
	return &twitterConcern{sm, new(extraKey), gofeed.NewParser()}
}
