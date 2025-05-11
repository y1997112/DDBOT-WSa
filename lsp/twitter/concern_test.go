package twitter

import (
	"github.com/cnxysoft/DDBOT-WSa/internal/test"
	"github.com/cnxysoft/DDBOT-WSa/lsp/concern"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTwitterConcern_GetUserInfo(t *testing.T) {
	tests := []struct {
		name        string
		screenName  string
		mockParser  func() *gofeed.Parser
		expected    *UserInfo
		expectError bool
	}{
		{
			name:       "successful user info fetch",
			screenName: "testuser",
			mockParser: func() *gofeed.Parser {
				return gofeed.NewParser()
			},
			expected: &UserInfo{
				Id:              "testuser",
				Name:            "test User",
				ProfileImageUrl: "https://nitter.poast.org/pic/pbs.twimg.com%2Fprofile_images%2F1889333234816659456%2FYm8bUTqX_400x400.jpg",
			},
			expectError: false,
		},
		{
			name:       "parser error",
			screenName: "erroruser",
			mockParser: func() *gofeed.Parser {
				return gofeed.NewParser()
			},
			expected:    nil,
			expectError: true,
		},
		{
			name:       "real api response",
			screenName: "peilien_vrc", // 使用真实存在的账号
			mockParser: func() *gofeed.Parser {
				// 使用真实网络请求
				return gofeed.NewParser()
			},
			expected: &UserInfo{
				Id:              "peilien_vrc",
				Name:            "ペイリアン💙🫧",
				ProfileImageUrl: "https://nitter.poast.org/pic/pbs.twimg.com%2Fprofile_images%2F1834361632975388672%2FNNRZqyz0_400x400.jpg",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test.InitBuntdb(t)
			defer test.CloseBuntdb(t)
			// 创建 twitterConcern 时注入 mock 解析器
			tc := &twitterConcern{
				twitterStateManager: &twitterStateManager{
					StateManager: concern.NewStateManagerWithStringID(Site, nil),
				},
				extraKey: new(extraKey),
				parser:   tt.mockParser(), // 添加 parser 字段到结构体
			}

			result, err := tc.FindUserInfo(tt.screenName, true)

			if tt.expectError {
				assert.Nil(t, err)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestTwitterConcern_GetTweets(t *testing.T) {
	// 创建测试用的HTML响应内容
	testHTML := `
    <html>
    <head>
		<title>test User (@testuser) / Twitter</title>
		<meta property="og:title" content="efbell (@YY749649883736)">
	</head>
    <body>
<div class="timeline-item ">
              <a class="tweet-link" href="/YY749649883736/status/1920439635265601673#m"></a>
              <div class="tweet-body">
                <div><div class="tweet-header">
                    <a class="tweet-avatar" href="/YY749649883736"><img class="avatar round" src="/pic/profile_images%2F1898789065090297856%2FRUoCd_rU_bigger.jpg" alt="" loading="lazy"></a>
                    <div class="tweet-name-row">
                      <div class="fullname-and-username">
                        <a class="fullname" href="/YY749649883736" title="efbell">efbell<div class="icon-container"><span class="icon-ok verified-icon blue" title="Verified blue account"></span></div></a>
                        <a class="username" href="/YY749649883736" title="@YY749649883736">@YY749649883736</a>
                      </div>
                      <span class="tweet-date"><a href="/YY749649883736/status/1920439635265601673#m" title="May 8, 2025 · 11:24 AM UTC">May 8</a></span>
                    </div>
                  </div></div>
                <div class="tweet-content media-body" dir="auto">ムツキ
<a href="/search?q=%23ブルアカ">#ブルアカ</a> <a href="/search?q=%23BlueArchive">#BlueArchive</a></div>
                <div class="attachments"><div class="gallery-row" style=""><div class="attachment image"><a class="still-image" href="/pic/orig/media%2FGqbFhUuW0AAVRk4.jpg" target="_blank"><img src="/pic/media%2FGqbFhUuW0AAVRk4.jpg%3Fname%3Dsmall%26format%3Dwebp" alt="" loading="lazy"></a></div></div></div>
                <div class="tweet-stats">
                  <span class="tweet-stat"><div class="icon-container"><span class="icon-comment" title=""></span> 5</div></span>
                  <span class="tweet-stat"><div class="icon-container"><span class="icon-retweet" title=""></span> 409</div></span>
                  <span class="tweet-stat"><div class="icon-container"><span class="icon-quote" title=""></span> 3</div></span>
                  <span class="tweet-stat"><div class="icon-container"><span class="icon-heart" title=""></span> 4,117</div></span>
                </div>
              </div>
            </div>
<div class="timeline-item ">
              <a class="tweet-link" href="/peace_maki02/status/1920766476270678132#m"></a>
              <div class="tweet-body">
                <div>
                  <div class="pinned"><span><div class="icon-container"><span class="icon-pin" title=""></span> Pinned Tweet</div></span></div>
                  <div class="tweet-header">
                    <a class="tweet-avatar" href="/peace_maki02"><img class="avatar round" src="/pic/profile_images%2F1362429533790498817%2FLURcNXBA_bigger.jpg" alt="" loading="lazy"></a>
                    <div class="tweet-name-row">
                      <div class="fullname-and-username">
                        <a class="fullname" href="/peace_maki02" title="安原宏和@９巻発売、アニメ化決定！🐨">安原宏和@９巻発売、アニメ化決定！🐨<div class="icon-container"><span class="icon-ok verified-icon blue" title="Verified blue account"></span></div></a>
                        <a class="username" href="/peace_maki02" title="@peace_maki02">@peace_maki02</a>
                      </div>
                      <span class="tweet-date"><a href="/peace_maki02/status/1920766476270678132#m" title="May 9, 2025 · 9:03 AM UTC">May 9</a></span>
                    </div>
                  </div>
                </div>
                <div class="tweet-content media-body" dir="auto">／
⋰
「ゲーセン少女と異文化交流」
🎮メインPVを公開🎮
⋱
＼

勘違いから始まる、
ゲーセンでの異文化交流👾🎀
TVアニメは7月6日(日)放送開始です！

リリー <a href="/search?q=%23天城サリー">#天城サリー</a>
蓮司 <a href="/search?q=%23千葉翔也">#千葉翔也</a>
葵衣 <a href="/search?q=%23小山内怜央">#小山内怜央</a>
花梨 <a href="/search?q=%23結川あさき">#結川あさき</a>
蛍 <a href="/search?q=%23石原夏織">#石原夏織</a>
桃子 <a href="/search?q=%23茅野愛衣">#茅野愛衣</a>

<a href="https://piped.video/watch?v=QOVabX4iYYY">piped.video/watch?v=QOVabX4i…</a>

<a href="/search?q=%23ゲーセン少女">#ゲーセン少女</a></div>
                <div class="attachments card"><div class="gallery-video"><div class="attachment video-container">
                      <video poster="/pic/amplify_video_thumb%2F1920766235832172544%2Fimg%2F-wvBuAXTdzu4CEnN.jpg%3Fname%3Dsmall%26format%3Dwebp" data-url="/video/CDBABB50751BD/https%3A%2F%2Fvideo.twimg.com%2Famplify_video%2F1920766235832172544%2Fpl%2FFNAXoec5cT40vEaG.m3u8" data-autoload="false"></video>
                      <div class="video-overlay" onclick="playVideo(this)">
                      <div class="overlay-circle"><span class="overlay-triangle"></span></div>
                      </div>
                    </div></div></div>
                <div class="tweet-stats">
                  <span class="tweet-stat"><div class="icon-container"><span class="icon-comment" title=""></span> 27</div></span>
                  <span class="tweet-stat"><div class="icon-container"><span class="icon-retweet" title=""></span> 1,782</div></span>
                  <span class="tweet-stat"><div class="icon-container"><span class="icon-quote" title=""></span> 216</div></span>
                  <span class="tweet-stat"><div class="icon-container"><span class="icon-heart" title=""></span> 4,329</div></span>
                  <span class="tweet-stat"><div class="icon-container"><span class="icon-play" title=""></span> 0</div></span>
                </div>
              </div>
            </div>
    </body>
    </html>
    `

	// 创建测试服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testHTML))
	}))
	defer ts.Close()

	// 替换 buildProfileURL 函数（需要修改 production 代码以支持此操作）
	originalBuildProfileURL := buildProfileURL
	buildProfileURL = func(screenName string) string {
		return ts.URL
	}
	defer func() { buildProfileURL = originalBuildProfileURL }()

	test.InitBuntdb(t)
	defer test.CloseBuntdb(t)

	// 初始化 twitterConcern
	tc := &twitterConcern{
		twitterStateManager: &twitterStateManager{
			StateManager: concern.NewStateManagerWithStringID(Site, nil),
		},
		extraKey: new(extraKey),
	}

	// 执行测试
	tweets, err := tc.GetTweets("testuser")

	assert.NoError(t, err)
	assert.NotNil(t, tweets)
	assert.Len(t, tweets, 2, "应解析出2条推文")

	// 验证推文内容
	tweet := tweets[0]
	assert.Equal(t, "1920439635265601673", tweet.ID)
	assert.Contains(t, tweet.Content, "ムツキ\n#ブルアカ #BlueArchive")
	assert.Equal(t, int64(4117), tweet.Likes)
	assert.Equal(t, int64(409), tweet.Retweets)
}
