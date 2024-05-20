package plugin

import (
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

func (p *Plugin) isUserConnected(userId string) (bool, error) {
	var encryptedToken []byte
	err := p.client.KV.Get(getUserTokenKey(userId), &encryptedToken)
	if err != nil {
		return false, err
	}
	if len(encryptedToken) == 0 {
		return false, nil
	}
	return true, nil
}

func (p *Plugin) handleConnect(c *plugin.Context, args *model.CommandArgs, parameters []string) string {
	if connected, err := p.isUserConnected(args.UserId); connected && err == nil {
		return "You have already connected your Google account. If you want to reconnect then disconnect the account first using `/drive disconnect`."
	}
	siteURL := p.client.Configuration.GetConfig().ServiceSettings.SiteURL
	if siteURL == nil {
		return "Encountered an error connecting to Google Drive."
	}

	return fmt.Sprintf("[Click here to link your Google account.](%s/plugins/%s/oauth/connect)", *siteURL, manifest.Id)
}