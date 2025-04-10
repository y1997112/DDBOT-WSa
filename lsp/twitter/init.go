package twitter

import "github.com/Sora233/DDBOT/lsp/concern"

func init() {
	concern.RegisterConcern(newConcern(concern.GetNotifyChan()))
}
