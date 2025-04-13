package bilibili

import (
	"github.com/Sora233/DDBOT/proxy_pool"
	"github.com/Sora233/DDBOT/requests"
	"github.com/Sora233/DDBOT/utils"
	"time"
)

const (
	PathRoomInfo = "/room/v1/Room/get_info"
)

type RoomInfoRequest struct {
	RoomId int64 `json:"room_id"`
}

func GetRoomInfo(roomId int64) (*LiveRoomData, error) {
	params, err := utils.ToDatas(&RoomInfoRequest{
		RoomId: roomId,
	})
	if err != nil {
		return nil, err
	}
	opts := []requests.Option{
		AddUAOption(),
		requests.TimeoutOption(time.Second * 15),
		requests.RequestAutoHostOption(),
		requests.ProxyOption(proxy_pool.PreferNone),
		requests.WithCookieJar(cj.Load()),
	}
	opts = append(opts, GetVerifyOption()...)
	var resp = new(LiveRoomData)
	err = requests.Get(BPath(PathRoomInfo), params, resp, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (l *LiveRoomData) GetLiveStatus() LiveStatus {
	if l.Data.LiveStatus == 1 {
		return LiveStatus_Living
	}
	return LiveStatus_NoLiving
}

func (l *LiveRoomData) GetCover() string {
	return l.Data.UserCover
}

func (l *LiveRoomData) GetTitle() string {
	return l.Data.Title
}
