package twitter

import (
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"net/url"
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
				// 方案一：使用 HTTP 测试服务器模拟响应
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte(`<rss><channel><title>Test User / @testuser</title></channel></rss>`))
				}))

				parser := gofeed.NewParser()
				parser.Client = &http.Client{Transport: &http.Transport{
					Proxy: func(*http.Request) (*url.URL, error) {
						return url.Parse(ts.URL)
					},
				}}
				return parser
			},
			expected: &UserInfo{
				Id:   "testuser",
				Name: "Test User",
			},
			expectError: false,
		},
		{
			name:       "parser error",
			screenName: "erroruser",
			mockParser: func() *gofeed.Parser {
				// 返回无效的XML结构触发解析错误
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte(`<invalid>xml`))
				}))

				parser := gofeed.NewParser()
				parser.Client = &http.Client{Transport: &http.Transport{
					Proxy: func(*http.Request) (*url.URL, error) {
						return url.Parse(ts.URL)
					},
				}}
				return parser
			},
			expected:    nil,
			expectError: true,
		},
		{
			name:       "title without slash",
			screenName: "simpleuser",
			mockParser: func() *gofeed.Parser {
				// 返回无斜杠的标题
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte(`<rss><channel><title>Simple User</title></channel></rss>`))
				}))

				parser := gofeed.NewParser()
				parser.Client = &http.Client{Transport: &http.Transport{
					Proxy: func(*http.Request) (*url.URL, error) {
						return url.Parse(ts.URL)
					},
				}}
				return parser
			},
			expected: &UserInfo{
				Id:   "simpleuser",
				Name: "Simple User",
			},
			expectError: false,
		},
		{
			name:       "real api response",
			screenName: "peilien_vrc", // 使用真实存在的账号
			mockParser: func() *gofeed.Parser {
				// 使用真实网络请求
				return gofeed.NewParser()
			},
			expected:    nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建 twitterConcern 时注入 mock 解析器
			tc := &twitterConcern{
				parser: tt.mockParser(), // 添加 parser 字段到结构体
			}
			result, _ := tc.GetUserInfo(tt.screenName)

			if tt.expectError {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
