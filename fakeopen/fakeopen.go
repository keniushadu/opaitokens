package fakeopen

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// wrap ai.fakeopen.com api
// https://ai.fakeopen.com/token
// https://ai.fakeopen.com/pool

const SharedTokenRegisterUrl = "https://ai.fakeopen.com/token/register"
const PooledTokenRegisterUrl = "https://ai.fakeopen.com/pool/update"
const SessionTokenGenACTUrl = "https://ai.fakeopen.com/auth/session"
const PooledTokensLimit = 100

var jar, _ = cookiejar.New(nil)

type AiFakeOpenPlatform struct {
	Client *http.Client
}

func NewAiFakeOpenPlatform() *AiFakeOpenPlatform {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		Proxy:           http.ProxyFromEnvironment,
	}
	client := &http.Client{
		Timeout:   time.Second * 100,
		Transport: tr,
		Jar:       jar,
	}
	platform := &AiFakeOpenPlatform{
		Client: client,
	}
	return platform
}

type SharedTokenReq struct {
	//唯一表示
	UniqueName string `url:"unique_name"`
	//openai token
	AccessToken string `url:"access_token"`
	//过期时间 默认为0 默认取0 表示使用accesstoken中的过期时间
	ExpiresIn int `url:"expires_in"`
	//限制使用范围 可以在那些域名下使用  为空表示不限制
	SiteLimit string `url:"site_limit"`
	//是否显示对话历史 默认为true
	ShowConversations bool `url:"show_conversations"`
}

type SharedToken struct {
	ExpireAt          int64  `json:"expire_at"`
	ShowConversations bool   `json:"show_conversations"`
	ShowUserinfo      bool   `json:"show_userinfo"`
	SiteLimit         string `json:"site_limit"`
	TokenKey          string `json:"token_key"`
	UniqueName        string `json:"unique_name"`
}

// GetSharedToken
//
//	@Description: 申请fakeopen fk
//	@receiver f
//	@param uniqueName
//	@param accessToken
//	@param expiresIn
//	@param siteLimit
//	@param showConversations
//	@return SharedToken
//	@return error
func (f *AiFakeOpenPlatform) GetSharedToken(shareTokenReq SharedTokenReq) (SharedToken, error) {
	token := SharedToken{}

	// Convert the struct to url.Values
	formValues := url.Values{}
	formValues.Set("unique_name", shareTokenReq.UniqueName)
	formValues.Set("access_token", shareTokenReq.AccessToken)
	formValues.Set("expires_in", strconv.Itoa(shareTokenReq.ExpiresIn))
	formValues.Set("site_limit", shareTokenReq.SiteLimit)
	formValues.Set("show_conversations", strconv.FormatBool(shareTokenReq.ShowConversations))

	// Send the form data as a POST request
	resp, err := f.Client.PostForm(SharedTokenRegisterUrl, formValues)
	if err != nil {
		return token, errors.New("get shared token failed: " + err.Error())
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return token, errors.New("read response failed: " + err.Error())
	}

	err = json.Unmarshal(body, &token)
	if err != nil {
		return token, errors.New("unmarshal response to json failed: " + err.Error())
	}
	return token, nil
}

// RevokeSharedToken
//
//	@Description: 撤销fakeopen fk
//	@receiver f
//	@param uniqueName
//	@param accessToken
//	@return SharedToken
//	@return error
func (f *AiFakeOpenPlatform) RevokeSharedToken(uniqueName string, accessToken string) (SharedToken, error) {
	req := SharedTokenReq{
		UniqueName:        uniqueName,
		AccessToken:       accessToken,
		ExpiresIn:         0,
		SiteLimit:         "",
		ShowConversations: true,
	}
	return f.GetSharedToken(req)
}

type PooledTokenReq struct {
	ShareTokens []string `json:"share_tokens"`
	PoolToken   string   `json:"pool_token"`
}

type PooledToken struct {
	Count     int    `json:"count"`
	PoolToken string `json:"pool_token"`
}

// RenewPooledToken
//
//	@Description: get or renew pool token by fk tokens
//	@receiver f
//	@param shareTokens
//	@param poolToken
//	@return PooledToken
//	@return error
func (f *AiFakeOpenPlatform) RenewPooledToken(pooledTokenReq PooledTokenReq) (PooledToken, error) {
	pToken := PooledToken{}
	if len(pooledTokenReq.ShareTokens) > PooledTokensLimit || len(pooledTokenReq.ShareTokens) == 0 {
		return pToken, errors.New("invalid share tokens, it must be less than 100 but greater than 0")
	}

	formValues := url.Values{}

	formValues.Set("share_tokens", strings.Join(pooledTokenReq.ShareTokens, "\n"))
	formValues.Set("pool_token", pooledTokenReq.PoolToken)

	// Send the form data as a POST request
	resp, err := f.Client.PostForm(PooledTokenRegisterUrl, formValues)
	if err != nil {
		return pToken, errors.New("get pooled token failed: " + err.Error())
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return pToken, errors.New("read response failed: " + err.Error())
	}

	err = json.Unmarshal(body, &pToken)
	if err != nil {
		return pToken, errors.New("unmarshal response to json failed: " + err.Error())
	}
	return pToken, nil

}

// SessionToken session token
// openai 中的session token 可以用来获取accessToken
// session token有效期为90 天
type SessionToken struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	SessionToken string `json:"session_token"`
	TokenType    string `json:"token_type"`
}

func (f *AiFakeOpenPlatform) GetAccessTokenBySessionToken(sessionTokenFromOpenai string) (SessionToken, error) {
	sessionToken := SessionToken{}
	formValues := url.Values{}

	formValues.Set("session_token", sessionTokenFromOpenai)

	// Send the form data as a POST request
	resp, err := f.Client.PostForm(SessionTokenGenACTUrl, formValues)
	if err != nil {
		return sessionToken, errors.New("get access token failed: " + err.Error())
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return sessionToken, errors.New("read response failed: " + err.Error())
	}

	err = json.Unmarshal(body, &sessionToken)
	if err != nil {
		return sessionToken, errors.New("unmarshal response to json failed: " + err.Error())
	}
	return sessionToken, nil
}
