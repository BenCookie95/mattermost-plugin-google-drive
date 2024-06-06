package plugin

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/option"
)

type WatchChannelData struct {
	ChannelId  string `json:"channel_id"`
	ResourceId string `json:"resource_id"`
	MMUserId   string `json:"mm_user_id"`
	Expiration int64  `json:"expiration"`
}

func (p *Plugin) handleAddedComment(dSrv *drive.Service, fileID, userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	commentID := activity.Targets[0].FileComment.LegacyCommentId
	dSrv.About.Get().Do()
	comment, _ := dSrv.Comments.Get(fileID, commentID).Fields("*").Do()
	quotedValue := ""
	if comment.QuotedFileContent != nil {
		quotedValue = comment.QuotedFileContent.Value
	}
	props := map[string]any{
		"attachments": []any{
			map[string]any{
				"pretext": fmt.Sprintf("%s commented on %s %s", comment.Author.DisplayName, getInlineImage("File icon:", file.IconLink), getHyperlink(file.Name, file.WebViewLink)),
				"text":    fmt.Sprintf("%s\n> %s", quotedValue, comment.Content),
				"actions": []any{
					map[string]any{
						"name": "Reply to comment",
						"integration": map[string]any{
							"url": fmt.Sprintf("%s/plugins/%s/api/v1/reply_dialog", *p.API.GetConfig().ServiceSettings.SiteURL, manifest.Id),
							"context": map[string]any{
								"commentId": commentID,
								"fileId":    fileID,
							},
						},
					},
				},
			},
		},
	}
	p.createBotDMPost(userID, "", props)
}

func (p *Plugin) handleDeletedComment(userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	urlToComment := activity.Targets[0].FileComment.LinkToDiscussion
	message := fmt.Sprintf("A comment was deleted in %s %s", getInlineImage("Google failed:", file.IconLink), getHyperlink(file.Name, urlToComment))
	p.createBotDMPost(userID, message, nil)
}

func (p *Plugin) handleReplyAdded(dSrv *drive.Service, fileID, userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	commentID := activity.Targets[0].FileComment.LegacyDiscussionId
	dSrv.About.Get().Do()
	comment, _ := dSrv.Comments.Get(fileID, commentID).Fields("*").IncludeDeleted(true).Do()
	urlToComment := activity.Targets[0].FileComment.LinkToDiscussion
	lastReply := ""
	onBeforeLast := ""
	if len(comment.Replies) > 0 {
		lastReply = comment.Replies[len(comment.Replies)-1].Content
		if len(comment.Replies) > 1 {
			onBeforeLast = comment.Replies[len(comment.Replies)-2].Content
		}
	}
	props := map[string]any{
		"attachments": []any{
			map[string]any{
				"pretext": fmt.Sprintf("%s replied on %s %s", comment.Author.DisplayName, getInlineImage("File icon:", file.IconLink), getHyperlink(file.Name, urlToComment)),
				"text":    fmt.Sprintf("Previous reply:\n%s\n> %s", onBeforeLast, lastReply),
				"actions": []any{
					map[string]any{
						"name": "Reply to comment",
						"integration": map[string]any{
							"url": fmt.Sprintf("%s/plugins/%s/api/v1/reply_dialog", *p.API.GetConfig().ServiceSettings.SiteURL, manifest.Id),
							"context": map[string]any{
								"commentId": commentID,
								"fileId":    fileID,
							},
						},
					},
				},
			},
		},
	}
	p.createBotDMPost(userID, "", props)
}

func (p *Plugin) handleReplyDeleted(userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	urlToComment := activity.Targets[0].FileComment.LinkToDiscussion
	message := fmt.Sprintf("A comment reply was deleted in %s %s", getInlineImage("Google failed:", file.IconLink), getHyperlink(file.Name, urlToComment))
	p.createBotDMPost(userID, message, nil)
}

func (p *Plugin) handleResolvedComment(dSrv *drive.Service, fileID, userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	commentID := activity.Targets[0].FileComment.LegacyCommentId
	dSrv.About.Get().Do()
	comment, _ := dSrv.Comments.Get(fileID, commentID).Fields("*").IncludeDeleted(true).Do()
	urlToComment := activity.Targets[0].FileComment.LinkToDiscussion
	message := fmt.Sprintf("%s marked a thread as resolved in %s %s", comment.Author.DisplayName, getInlineImage("File icon:", file.IconLink), getHyperlink(file.Name, urlToComment))
	p.createBotDMPost(userID, message, nil)
}

func (p *Plugin) handleReopenedComment(dSrv *drive.Service, fileID, userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	commentID := activity.Targets[0].FileComment.LegacyDiscussionId
	dSrv.About.Get().Do()
	comment, _ := dSrv.Comments.Get(fileID, commentID).Fields("*").IncludeDeleted(true).Do()
	urlToComment := activity.Targets[0].FileComment.LinkToDiscussion
	message := fmt.Sprintf("%s reopened a thread in %s %s", comment.Author.DisplayName, getInlineImage("File icon:", file.IconLink), getHyperlink(file.Name, urlToComment))
	p.createBotDMPost(userID, message, nil)
}

func (p *Plugin) handleSuggestionReplyAdded(userID string, activity *driveactivity.DriveActivity, file *drive.File) {
	urlToComment := activity.Targets[0].FileComment.LinkToDiscussion
	message := fmt.Sprintf("%s added a new suggestion in %s %s", file.LastModifyingUser.DisplayName, getInlineImage("File icon:", file.IconLink), getHyperlink(file.Name, urlToComment))
	p.createBotDMPost(userID, message, nil)
}

func (p *Plugin) handleCommentNotifications(fileID, userID string, activity *driveactivity.DriveActivity, authToken *oauth2.Token) {
	ctx := context.Background()
	conf := p.getOAuthConfig()
	dSrv, _ := drive.NewService(ctx, option.WithTokenSource(conf.TokenSource(ctx, authToken)))
	file, _ := dSrv.Files.Get(fileID).Fields("webViewLink", "id", "permissions", "name", "iconLink", "createdTime").Do()

	postSubType := activity.PrimaryActionDetail.Comment.Post.Subtype

	switch postSubType {
	case "ADDED":
		p.handleAddedComment(dSrv, fileID, userID, activity, file)
	case "DELETED":
		p.handleDeletedComment(userID, activity, file)
	case "REPLY_ADDED":
		p.handleReplyAdded(dSrv, fileID, userID, activity, file)
	case "REPLY_DELETED":
		p.handleReplyDeleted(userID, activity, file)
	case "RESOLVED":
		p.handleResolvedComment(dSrv, fileID, userID, activity, file)
	case "REOPENED":
		p.handleReopenedComment(dSrv, fileID, userID, activity, file)
	}

	suggestion := activity.PrimaryActionDetail.Comment.Suggestion
	if suggestion == nil {
		return
	}
	suggestionSubType := suggestion.Subtype

	switch suggestionSubType {
	case "REPLY_ADDED":
		p.handleSuggestionReplyAdded(userID, activity, file)
	}
}

func (p *Plugin) handleFileSharedNotification(fileID, userID string, authToken *oauth2.Token) {
	ctx := context.Background()
	conf := p.getOAuthConfig()
	dSrv, _ := drive.NewService(ctx, option.WithTokenSource(conf.TokenSource(ctx, authToken)))
	file, _ := dSrv.Files.Get(fileID).Fields("webViewLink", "id", "permissions", "name", "iconLink", "createdTime").Do()

	author := file.SharingUser
	userDisplay := p.getUserDisplayName(author)
	message := userDisplay + " shared an item with you"

	p.createBotDMPost(userID, message, map[string]any{
		"attachments": []any{map[string]any{
			"title":       file.Name,
			"title_link":  file.WebViewLink,
			"footer":      "Google Drive for Mattermost",
			"footer_icon": file.IconLink,
		}},
	})
}

func (p *Plugin) startDriveWatchChannel(userId, resourceId, channelId string) error {
	ctx := context.Background()
	conf := p.getOAuthConfig()
	authToken, err := p.getGoogleUserToken(userId)
	if err != nil {
		p.API.LogError("failed to get auth token", "err", err)
		return err
	}

	srv, err := drive.NewService(ctx, option.WithTokenSource(conf.TokenSource(ctx, authToken)))
	if err != nil {
		p.API.LogError("failed to create drive service client", "err", err)
		return err
	}

	startPageToken, err := srv.Changes.GetStartPageToken().Do()
	if err != nil {
		p.API.LogError("failed to get start page token", "err", err)
		return err
	}

	url, err := url.Parse(fmt.Sprintf("%s/plugins/%s/api/v1/webhook", *p.client.Configuration.GetConfig().ServiceSettings.SiteURL, manifest.Id))
	if err != nil {
		p.API.LogError("failed to parse webhook url", "err", err)
		return err
	}
	query := url.Query()
	query.Add("userId", userId)
	url.RawQuery = query.Encode()

	requestChannel := drive.Channel{
		Kind:       "api#channel",
		Address:    url.String(),
		Payload:    true,
		Id:         uuid.NewString(),
		Type:       "web_hook",
		Expiration: time.Now().Add(604800 * time.Second).UnixMilli(),
		Params: map[string]string{
			"userId": userId,
		},
	}
	if channelId != "" {
		requestChannel.Id = channelId
	}
	if resourceId != "" {
		requestChannel.ResourceId = resourceId
	}

	channel, err := srv.Changes.Watch(startPageToken.StartPageToken, &requestChannel).Do()
	if err != nil {
		p.API.LogError("failed to register watch on drive", "err", err)
		return err
	}

	channelData := WatchChannelData{
		ChannelId:  channel.Id,
		ResourceId: channel.ResourceId,
		Expiration: channel.Expiration,
	}
	_, err = p.client.KV.Set(getWatchChannelDataKey(userId), channelData)
	if err != nil {
		p.API.LogError("failed to set drive change channel data", "userId", userId, "channelData", channelData)
		return err
	}
	return nil
}

func (p *Plugin) startDriveActivityNotifications(userId string) string {
	err := p.startDriveWatchChannel(userId, "", "")
	if err != nil {
		return "Something went wrong while starting Drive activity notifications. Please contact your organization admin for support."
	}

	return "Successfully enabled drive activity notifications."
}

func (p *Plugin) stopDriveActivityNotifications(userID string) string {
	var watchChannelData WatchChannelData
	err := p.client.KV.Get(getWatchChannelDataKey(userID), &watchChannelData)
	if err != nil {
		p.API.LogError("failed to get drive change channel data", "userId", userID)
		return "Something went wrong while stopping Drive activity notifications. Please contact your organization admin for support."
	}

	ctx := context.Background()
	conf := p.getOAuthConfig()
	authToken, _ := p.getGoogleUserToken(userID)
	srv, _ := drive.NewService(ctx, option.WithTokenSource(conf.TokenSource(ctx, authToken)))

	err = srv.Channels.Stop(&drive.Channel{
		Id:         watchChannelData.ChannelId,
		ResourceId: watchChannelData.ResourceId,
	}).Do()

	if err != nil {
		p.API.LogError("failed to stop drive change channel", "err", err)
		return "Something went wrong while stopping Drive activity notifications. Please contact your organization admin for support."
	}

	return "Successfully disabled drive activity notifications."
}

func (p *Plugin) handleNotifications(c *plugin.Context, args *model.CommandArgs, parameters []string) string {
	subcommand := parameters[0]

	allowedCommands := []string{"start", "stop"}
	if !slices.Contains(allowedCommands, subcommand) {
		return fmt.Sprintf("%s is not a valid notifications subcommand", subcommand)
	}

	switch subcommand {
	case "start":
		return p.startDriveActivityNotifications(args.UserId)
	case "stop":
		return p.stopDriveActivityNotifications(args.UserId)
	}
	return ""
}
