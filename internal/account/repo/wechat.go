package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	. "knowledge-base/internal/account/entity"
)

const wechatCodeSessionURL = "https://api.weixin.qq.com/sns/jscode2session"

type HTTPWeChatClient struct {
	appID      string
	secret     string
	httpClient *http.Client
}

func NewHTTPWeChatClient(appID, secret string, client *http.Client) *HTTPWeChatClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPWeChatClient{appID: appID, secret: secret, httpClient: client}
}

func (client *HTTPWeChatClient) ExchangeCode(ctx context.Context, code string) (WeChatIdentity, error) {
	query := url.Values{
		"appid":      {client.appID},
		"secret":     {client.secret},
		"js_code":    {code},
		"grant_type": {"authorization_code"},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, wechatCodeSessionURL+"?"+query.Encode(), nil)
	if err != nil {
		return WeChatIdentity{}, err
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return WeChatIdentity{}, ErrWeChatLogin
	}
	defer response.Body.Close()
	var payload struct {
		OpenID  string `json:"openid"`
		UnionID string `json:"unionid"`
		ErrCode int    `json:"errcode"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return WeChatIdentity{}, ErrWeChatLogin
	}
	if response.StatusCode != http.StatusOK || payload.ErrCode != 0 || payload.OpenID == "" {
		return WeChatIdentity{}, fmt.Errorf("%w: code=%d", ErrWeChatLogin, payload.ErrCode)
	}
	return WeChatIdentity{AppID: client.appID, OpenID: payload.OpenID, UnionID: payload.UnionID}, nil
}
