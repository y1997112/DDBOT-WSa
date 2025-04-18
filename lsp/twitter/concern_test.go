package twitter

import (
	"github.com/cnxysoft/DDBOT-WSa/internal/test"
	"github.com/cnxysoft/DDBOT-WSa/lsp/concern"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
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
