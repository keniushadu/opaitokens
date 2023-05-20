package opaitokens

import (
	"fmt"
	"testing"
)

func TestUserCase(t *testing.T) {
	email := "xxx@example.com"
	password := "xxxxxxx"

	tokens := NewOpaiTokens(email, password)
	token := tokens.FetchToken()
	fmt.Printf("token info: %v\n", token)
	accessToken := token.OpenaiToken.AccessToken
	// use the access token
	fmt.Println("i am using access token: %v\n", accessToken)

	token = tokens.RefreshToken()
	fmt.Println("token info again: %v\n", token)
	accessToken = token.RefreshedToken.AccessToken
	//use the refresh token
	fmt.Println("i am using refresh token: %", accessToken)

}
