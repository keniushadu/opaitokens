package auth

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fireinrain/opaitokens/utils"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// 创建一个cookie jar
var jar, _ = cookiejar.New(nil)

type Auth0 struct {
	sessionToken string
	email        string
	password     string
	useCache     bool
	mfa          string
	session      *http.Client
	reqHeaders   http.Header
	accessToken  string
	refreshToken string
	expires      time.Time
	userAgent    string
	authForCode  bool
}

func NewAuth0(email, password string, mfa string, useCache bool) *Auth0 {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		Proxy:           http.ProxyFromEnvironment,
	}

	auth := &Auth0{
		sessionToken: "",
		email:        email,
		password:     password,
		mfa:          mfa,
		useCache:     useCache,
		session: &http.Client{
			Timeout:   time.Second * 100,
			Transport: tr,
			Jar:       jar,
		},
		reqHeaders: http.Header{
			"User-Agent": []string{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36"},
		},
		accessToken: "",
		expires:     time.Time{},
		userAgent:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36",
		authForCode: false,
	}
	return auth
}

func (a *Auth0) checkEmail(email string) bool {
	re := regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,7}\b`)
	return re.MatchString(email)
}

func (a *Auth0) Auth(loginLocal bool) (string, error) {
	if a.useCache && a.accessToken != "" && a.expires.After(time.Now()) {
		return a.accessToken, nil
	}

	if !a.checkEmail(a.email) || a.password == "" {
		return "", errors.New("invalid email or password")
	}

	if loginLocal {
		return a.partOne()
	}

	return a.getAccessTokenProxy()
}

// GetRefreshToken
//
//	@Description: 返回refreshToken
//	@receiver a
//	@return string
func (a *Auth0) GetRefreshToken() string {
	return a.refreshToken
}

// DefaultApiPrefix
//
//	@Description: 获取默认的fakeopen网关地址
//	@receiver a
//	@return string
func (a *Auth0) DefaultApiPrefix() string {
	// Get the current date and subtract one day
	yesterday := time.Now().Add(-24 * time.Hour)

	// Format the date in 'YYYYMMDD' format
	dateStr := yesterday.Format("20060102")

	// Create the URL using the formatted date
	url := fmt.Sprintf("https://ai-%s.fakeopen.com", dateStr)
	return url
}

// AuthForCodeUrl
//
//	@Description: 获取回调code 和回调url
//	@receiver a
//	@param loginLocal
//	@return string
//	@return error
func (a *Auth0) AuthForCodeUrl() (string, error) {
	a.authForCode = true
	if a.useCache && a.accessToken != "" && a.expires.After(time.Now()) {
		return a.accessToken, nil
	}

	if !a.checkEmail(a.email) || a.password == "" {
		return "", errors.New("invalid email or password")
	}

	return a.partOne()

}

// partOne
//
//	@Description: 获取preauth 参数
//	@receiver a
//	@return string
//	@return error
func (a *Auth0) partOne() (string, error) {
	preauthUrl := fmt.Sprintf("%s/auth/preauth", a.DefaultApiPrefix())
	req, err := http.NewRequest("GET", preauthUrl, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	a.session.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := a.session.Do(req)
	if err != nil {
		return "", fmt.Errorf("error fetch preauth code in: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		var preauth struct {
			PreauthCookie string `json:"preauth_cookie"`
		}
		err := json.NewDecoder(resp.Body).Decode(&preauth)
		if err != nil {
			return "", fmt.Errorf("error decoding response: %v", err)
		}
		if preauth.PreauthCookie == "" {
			return "", fmt.Errorf("get preauth cookie failed")
		}
		return a.partTwo(preauth.PreauthCookie)
	}
	return "", errors.New("error request preauth code")
}

func (a *Auth0) partTwo(preauth string) (string, error) {
	codeVerifier := utils.GenerateCodeVerifier()
	codeChallenge := utils.GenerateCodeChallenge(codeVerifier)
	//codeChallenge := "w6n3Ix420Xhhu-Q5-mOOEyuPZmAsJHUbBpO8Ub7xBCY"
	//codeVerifier := "yGrXROHx_VazA0uovsxKfE263LMFcrSrdm4SlC-rob8"
	encodedString := "https://auth0.openai.com/authorize?client_id=pdlLIX2Y72MIl2rhLhTE9VV9bN905kBh&" +
		"audience=https%3A%2F%2Fapi.openai.com%2Fv1&redirect_uri=com.openai.chat%3A%2F%2Fauth0.openai.com%2Fios%2Fcom.openai.chat%2Fcallback&" +
		"scope=openid%20email%20profile%20offline_access%20model.request%20model.read%20organization.read%20offline&response_type=code&" +
		"code_challenge=w6n3Ix420Xhhu-Q5-mOOEyuPZmAsJHUbBpO8Ub7xBCY&code_challenge_method=S256&prompt=login&preauth_cookie=sample_preauth_cookie"
	//fmt.Println("decoded string:", decodedString)
	re := regexp.MustCompile(`code_challenge=[^&]+`)
	replacement := "code_challenge=" + codeChallenge
	newUrl := re.ReplaceAllString(encodedString, replacement)
	newUrl = strings.ReplaceAll(newUrl, "sample_preauth_cookie", preauth)
	return a.partThree(codeVerifier, newUrl)
}

func (a *Auth0) partThree(codeVerifier, urlStr string) (string, error) {
	headers := http.Header{}
	headers.Set("User-Agent", a.userAgent)
	headers.Set("Referer", "https://ios.chat.openai.com/")

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header = headers
	a.session.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return nil
	}
	resp, err := a.session.Do(req)
	if err != nil {
		return "", fmt.Errorf("error requesting login url: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {

		urlParams, err := url.ParseQuery(resp.Request.URL.RawQuery)
		if err != nil {
			return "", fmt.Errorf("error parsing url query: %v", err)
		}

		state := urlParams.Get("state")
		if state == "" {
			return "", errors.New("state parameter not found")
		}

		return a.partFour(codeVerifier, state)
	}

	return "", fmt.Errorf("error requesting login url: %v", err)
}

func (a *Auth0) partFour(codeVerifier, state string) (string, error) {
	urlStr := "https://auth0.openai.com/u/login/identifier?state=" + state
	headers := http.Header{}
	headers.Set("User-Agent", a.userAgent)
	headers.Set("Referer", urlStr)
	headers.Set("Origin", "https://auth0.openai.com")
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	data := url.Values{
		"state":                       {state},
		"username":                    {a.email},
		"js-available":                {"true"},
		"webauthn-available":          {"true"},
		"is-brave":                    {"false"},
		"webauthn-platform-available": {"false"},
		"action":                      {"default"},
	}

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header = headers
	//set not allow redirect
	a.session.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := a.session.Do(req)
	if err != nil {
		return "", fmt.Errorf("error checking email: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound {
		return a.partFive(codeVerifier, state)
	}

	return "", errors.New("error checking email")
}

func (a *Auth0) partFive(codeVerifier, state string) (string, error) {
	urlStr := "https://auth0.openai.com/u/login/password?state=" + state
	headers := http.Header{}
	headers.Set("User-Agent", a.userAgent)
	headers.Set("Referer", urlStr)
	headers.Set("Origin", "https://auth0.openai.com")
	headers.Set("Content-Type", "application/x-www-form-urlencoded")

	data := url.Values{
		"state":    {state},
		"username": {a.email},
		"password": {a.password},
		"action":   {"default"},
	}

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header = headers
	a.session.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := a.session.Do(req)
	if err != nil {
		return "", fmt.Errorf("error logging in: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound {
		location := resp.Header.Get("Location")
		if !strings.HasPrefix(location, "/authorize/resume?") {
			return "", errors.New("login callback failed")
		}
		return a.partSix(codeVerifier, location, urlStr)
	} else if resp.StatusCode == http.StatusBadRequest {
		return "", errors.New("wrong email or password")
	}

	return "", errors.New("error logging in")
}

func (a *Auth0) partSix(codeVerifier, location, urlStr string) (string, error) {
	url := "https://auth0.openai.com" + location
	headers := http.Header{}
	headers.Set("User-Agent", a.userAgent)
	headers.Set("Referer", urlStr)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header = headers
	a.session.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := a.session.Do(req)
	if err != nil {
		return "", fmt.Errorf("error logging in: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound {
		location := resp.Header.Get("Location")
		if strings.HasPrefix(location, "/u/mfa-otp-challenge?") {
			if a.mfa == "" {
				return "", fmt.Errorf("mfa string not found")
			}
			return a.partSeven(codeVerifier, location)
		}
		if !strings.HasPrefix(location, "com.openai.chat://auth0.openai.com/ios/com.openai.chat/callback?") {
			return "", fmt.Errorf("login callback failed")
		}
		return a.getAccessToken(codeVerifier, resp.Header.Get("Location"))
	}
	return "", errors.New("error logging in")
}

func (a *Auth0) partSeven(codeVerifier string, location string) (string, error) {
	urlStr := "https://auth0.openai.com" + location
	headers := http.Header{}
	headers.Set("User-Agent", a.userAgent)
	headers.Set("Referer", urlStr)
	headers.Set("Origin", "https://auth0.openai.com")

	u, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("error parsing url: %v", err)
	}

	// Get the raw query
	rawQuery := u.RawQuery
	urlParams, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", fmt.Errorf("error parsing url query: %v", err)
	}

	state := urlParams.Get("state")
	if state == "" {
		return "", errors.New("state parameter not found")
	}
	data := url.Values{
		"state":  {state},
		"code":   {a.mfa},
		"action": {"default"},
	}

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header = headers
	a.session.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := a.session.Do(req)
	if err != nil {
		return "", fmt.Errorf("error logging in: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound {
		location := resp.Header.Get("Location")
		if !strings.HasPrefix(location, "/authorize/resume?") {
			return "", errors.New("login with mfa callback failed")
		}
		return a.partSix(codeVerifier, location, urlStr)
	}
	if resp.StatusCode == http.StatusBadRequest {
		return "", errors.New("wrong mfa code for login")
	}
	return "", errors.New("login failed with wrong email or password")

}

func (a *Auth0) getAccessToken(codeVerifier, callbackURL string) (string, error) {

	u, err := url.Parse(callbackURL)
	if err != nil {
		return "", fmt.Errorf("error parsing callback url: %v", err)
	}

	urlParams := u.Query()
	if errorParam := urlParams.Get("error"); errorParam != "" {
		errorDesc := urlParams.Get("error_description")
		return "", fmt.Errorf("%s: %s", errorParam, errorDesc)
	}

	code := urlParams.Get("code")
	if code == "" {
		return "", fmt.Errorf("error getting code from callback url: %v", callbackURL)
	}
	//判断是否返回codeVerifier 和 code
	if a.authForCode {
		return codeVerifier + "|" + code, nil
	}

	urlStr := "https://auth0.openai.com/oauth/token"
	data := url.Values{
		"redirect_uri":  {"com.openai.chat://auth0.openai.com/ios/com.openai.chat/callback"},
		"grant_type":    {"authorization_code"},
		"client_id":     {"pdlLIX2Y72MIl2rhLhTE9VV9bN905kBh"},
		"code":          {code},
		"code_verifier": {codeVerifier},
	}

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	headers := http.Header{}
	headers.Set("User-Agent", a.userAgent)
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header = headers

	a.session.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := a.session.Do(req)
	if err != nil {
		return "", fmt.Errorf("error getting access token: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var response struct {
			AccessToken  string `json:"access_token"`
			ExpiresIn    int    `json:"expires_in"`
			RefreshToken string `json:"refresh_token"`
		}
		err := json.NewDecoder(resp.Body).Decode(&response)
		if err != nil {
			return "", fmt.Errorf("error decoding response: %v", err)
		}

		a.accessToken = response.AccessToken
		a.refreshToken = response.RefreshToken
		expiresAt := time.Now().UTC().Add(time.Second * time.Duration(response.ExpiresIn)).Add(-5 * time.Minute)
		a.expires = expiresAt
		return a.accessToken, nil
	}

	return "", fmt.Errorf("error getting access token: %s", resp.Status)
}

// TODO can't use, report 500 error
func (a *Auth0) getAccessTokenProxy() (string, error) {
	urlStr := fmt.Sprintf("%s/auth/login", a.DefaultApiPrefix())
	headers := http.Header{}
	headers.Set("User-Agent", a.userAgent)
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	data := url.Values{
		"username": {a.email},
		"password": {a.password},
		"mfa_code": {a.mfa},
	}

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header = headers
	a.session.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := a.session.Do(req)
	if err != nil {
		return "", fmt.Errorf("error getting access token: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var response struct {
			AccessToken  string `json:"access_token"`
			ExpiresIn    int    `json:"expires_in"`
			RefreshToken string `json:"refresh_token"`
		}
		err := json.NewDecoder(resp.Body).Decode(&response)
		if err != nil {
			return "", fmt.Errorf("error decoding response: %v", err)
		}

		a.accessToken = response.AccessToken
		a.refreshToken = response.RefreshToken
		expiresAt := time.Now().UTC().Add(time.Second * time.Duration(response.ExpiresIn)).Add(-5 * time.Minute)
		a.expires = expiresAt
		return a.accessToken, nil
	}

	return "", fmt.Errorf("error getting access token: %s", resp.Status)
}
