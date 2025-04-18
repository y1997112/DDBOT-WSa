package twitter

import "github.com/cnxysoft/DDBOT-WSa/lsp/concern"

func init() {
	concern.RegisterConcern(newConcern(concern.GetNotifyChan()))
}
