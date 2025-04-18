package bilibili

import (
	"fmt"
	"testing"

	"github.com/cnxysoft/DDBOT-WSa/utils"
)

func TestSignWbi(t *testing.T) {

	params, _ := utils.ToDatas(&XSpaceAccInfoRequest{
		Mid:           28278764,
		Token:         "",
		Platform:      "web",
		WebLocation:   "1550101",
		DmImgList:     "[]",
		DmImgStr:      "V2ViR0wgMS4wIChPcGVuR0wgRVMgMi4wIENocm9taXVtKQ",
		DmCoverImgStr: "QU5HTEUgKE5WSURJQSwgTlZJRElBIEdlRm9yY2UgUlRYIDMwOTAgKDB4MDAwMDIyMDQpIERpcmVjdDNEMTEgdnNfNV8wIHBzXzVfMCwgRDNEMTEpR29vZ2xlIEluYy4gKE5WSURJQS",
		DmImgInter:    `{"ds":[],"wh":[4541,3287,113],"of":[20,40,20]}`,
		WWebid:        "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzcG1faWQiOiIwLjAiLCJidXZpZCI6IjNFOEM0MTZFLUMzOTAtMkQ2RC1GOUUzLTEwRkNGODJFMEIwMzE4ODY2aW5mb2MiLCJ1c2VyX2FnZW50IjoiTW96aWxsYS81LjAgKFdpbmRvd3MgTlQgMTAuMDsgV2luNjQ7IHg2NCkgQXBwbGVXZWJLaXQvNTM3LjM2IChLSFRNTCwgbGlrZSBHZWNrbykgQ2hyb21lLzEyOS4wLjAuMCBTYWZhcmkvNTM3LjM2IEVkZy8xMjkuMC4wLjAiLCJidXZpZF9mcCI6IjdkZWJlYzQyNjI4ZGY5MWEwNGY2ZmMxMWI4OGY4NDY0IiwiYmlsaV90aWNrZXQiOiIzNTlmYTQ4MTdlZGE1ZDA1MzhlNjI2ZWZiNmViNmYxOCIsImNyZWF0ZWRfYXQiOjE3MjkwOTUzODEsInR0bCI6ODY0MDAsInVybCI6Ii8yODI3ODc2NCIsInJlc3VsdCI6Im5vcm1hbCIsImlzcyI6ImdhaWEiLCJpYXQiOjE3MjkwOTUzODF9.lEPY4x628rNXOKXjBpm97LGMu_omxQdUhE98Hm-RyS6LfpZuByx5FMFNsyRZCTk4MnmSaMwIEtSWGM7UM5fhefhOW5HtCKARZb0vWrb-J-73wvVVc21a8PrezzMhfgKg4yyK00F8tD68q92TFugwZyB94c0wTwNh-4S4Ry37Pth-muFXTsHjZOpDKcgjhw4SIROU9j8oHO8t-_2D8jwyKEDwlehgNdIAGTUpBAkyG2-hJ9yTnM6LFWGGzw06MrpR0MqmzxWlCQ_xfbO_sGWXuXJdGM_5u9YNO3FamMvmmfjTXOZ8NncCmXNRG8rpCg1c6nrOUvbZY7wklvh1JcvMtA",
	})

	// 调用函数进行签名
	signedParams := signWbi(params)

	// 输出签名的结果
	fmt.Println("签名结果:", signedParams)
}
