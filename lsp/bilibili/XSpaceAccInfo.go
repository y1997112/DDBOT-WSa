package bilibili

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"time"

	"github.com/cnxysoft/DDBOT-WSa/proxy_pool"
	"github.com/cnxysoft/DDBOT-WSa/requests"
	"github.com/cnxysoft/DDBOT-WSa/utils"
	"go.uber.org/atomic"
	"golang.org/x/net/html"
)

const (
	PathXSpaceAccInfo = "/x/space/wbi/acc/info"
)

type XSpaceAccInfoRequest struct {
	Mid           int64  `json:"mid"`
	Platform      string `json:"platform"`
	Token         string `json:"token"`
	WebLocation   string `json:"web_location"`
	DmImgList     string `json:"dm_img_list"`
	DmImgInter    string `json:"dm_img_inter"`
	DmCoverImgStr string `json:"dm_cover_img_str"`
	DmImgStr      string `json:"dm_img_str"`
	WWebid        string `json:"w_webid"`
}

var cj atomic.Pointer[cookiejar.Jar]

func refreshCookieJar() {
	j, _ := cookiejar.New(nil)
	err := requests.Get("https://www.bilibili.com/", nil, io.Discard,
		requests.WithCookieJar(j),
		AddUAOption(),
		requests.RequestAutoHostOption(),
		requests.HeaderOption("accept", "application/json"),
		requests.HeaderOption("accept-language", "zh-CN,zh;q=0.9"),
	)
	if err != nil {
		logger.Errorf("bilibili: refreshCookieJar request error %v", err)
	}
	cj.Store(j)
}

func XSpaceAccInfo(mid int64) (*XSpaceAccInfoResponse, error) {
	st := time.Now()
	defer func() {
		ed := time.Now()
		logger.WithField("FuncName", utils.FuncName()).Tracef("cost %v", ed.Sub(st))
	}()
	const headerOrigin = "https://space.bilibili.com"
	const headerAcceptLanguage = "zh-CN,zh;q=0.9"
	const headerUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	const headerBuvId3 = "55F2E775-090F-EC53-DDED-6FDD87687DAA76234infoc"
	const headerBNut = "1716350676"
	const headerBLsid = "5641F797_18F9E78EF04"
	const headerUuid = "F1BD8FB10-9651-10596-DA37-1CEC9A469A5376746infoc"
	type SpaceAccessIdResponse struct {
		AccessId string `json:"access_id"`
	}
	webUrl := headerOrigin + "/" + strconv.FormatInt(mid, 10)
	req, err := http.NewRequest("GET", webUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", headerAcceptLanguage)
	req.Header.Set("Origin", headerOrigin)
	req.Header.Set("Referer", webUrl)
	req.Header.Set("User-Agent", headerUserAgent)
	req.Header.Set("buvid3", headerBuvId3)
	req.Header.Set("b_nut", headerBNut)
	req.Header.Set("b_lsid", headerBLsid)
	req.Header.Set("_uuid", headerUuid)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, err
	}
	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}
	var scriptContent string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "head" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "script" {
					for _, attr := range c.Attr {
						if attr.Key == "id" && attr.Val == "__RENDER_DATA__" {
							if c.FirstChild != nil {
								scriptContent = c.FirstChild.Data
							}
						}
					}
				}
				f(c)
			}
		} else {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				f(c)
			}
		}
	}
	f(doc)
	if scriptContent == "" {
		return nil, fmt.Errorf("no script tag with id '__RENDER_DATA__' found")
	}
	scriptContent, err = url.QueryUnescape(scriptContent)
	if err != nil {
		return nil, err
	}
	var accResp SpaceAccessIdResponse
	err = json.Unmarshal([]byte(scriptContent), &accResp)
	if err != nil {
		return nil, err
	}
	accInfoUrl := BPath(PathXSpaceAccInfo)
	params, err := utils.ToDatas(&XSpaceAccInfoRequest{
		Mid:           mid,
		DmImgList:     "",
		Platform:      "web",
		WebLocation:   "1550101",
		DmImgStr:      "V2ViR0wgMS4wIChPcGVuR0wgRVMgMi4wIENocm9taXVtKQ",
		DmCoverImgStr: "QU5HTEUgKE5WSURJQSwgTlZJRElBIEdlRm9yY2UgUlRYIDMwOTAgKDB4MDAwMDIyMDQpIERpcmVjdDNEMTEgdnNfNV8wIHBzXzVfMCwgRDNEMTEpR29vZ2xlIEluYy4gKE5WSURJQS",
		DmImgInter:    `{"ds":[],"wh":[0,0,0],"of":[0,0,0]}`,
		WWebid:        accResp.AccessId,
	})
	if err != nil {
		return nil, err
	}
	signWbi(params)
	var opts = []requests.Option{
		requests.ProxyOption(proxy_pool.PreferNone),
		requests.TimeoutOption(time.Second * 15),
		AddUAOption(),
		requests.HeaderOption("Accept", "application/json"),
		requests.HeaderOption("Accept-language", headerAcceptLanguage),
		requests.HeaderOption("Origin", headerOrigin),
		requests.HeaderOption("Referer", headerOrigin+"/"+strconv.FormatInt(mid, 10)),
		requests.HeaderOption("User-Agent", headerUserAgent),
		//requests.CookieOption("buvid3", headerBuvId3),
		//requests.CookieOption("b_nut", headerBNut),
		requests.CookieOption("b_lsid", headerBLsid),
		requests.CookieOption("_uuid", headerUuid),
		requests.RequestAutoHostOption(),
		requests.WithCookieJar(cj.Load()),
		requests.NotIgnoreEmptyOption(),
		delete412ProxyOption,
	}
	opts = append(opts, GetVerifyOption()...)
	xsai := new(XSpaceAccInfoResponse)
	err = requests.Get(accInfoUrl, params, xsai, opts...)
	if err != nil {
		return nil, err
	}
	return xsai, nil
}
