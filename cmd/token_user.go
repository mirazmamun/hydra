/*
 * Copyright © 2015-2018 Aeneas Rekkas <aeneas+oss@aeneas.io>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * @author		Aeneas Rekkas <aeneas+oss@aeneas.io>
 * @copyright 	2015-2018 Aeneas Rekkas <aeneas+oss@aeneas.io>
 * @license 	Apache-2.0
 */

package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/ory/hydra/pkg"
	"github.com/ory/hydra/rand/sequence"
	"github.com/spf13/cobra"
	"github.com/toqueteos/webbrowser"
	"golang.org/x/oauth2"
)

// tokenUserCmd represents the token command
var tokenUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Generate an OAuth2 token using the code flow",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		if ok, _ := cmd.Flags().GetBool("skip-tls-verify"); ok {
			// fmt.Println("Warning: Skipping TLS Certificate Verification.")
			ctx = context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}})
		}

		scopes, _ := cmd.Flags().GetStringSlice("scopes")
		clientId, _ := cmd.Flags().GetString("id")
		clientSecret, _ := cmd.Flags().GetString("secret")
		redirectUrl, _ := cmd.Flags().GetString("redirect")
		backend, _ := cmd.Flags().GetString("token-url")
		frontend, _ := cmd.Flags().GetString("auth-url")

		if clientId == "" {
			clientId = c.ClientID
		}
		if clientSecret == "" {
			clientSecret = c.ClientSecret
		}
		if backend == "" {
			backend = pkg.JoinURLStrings(c.ClusterURL, "/oauth2/token")
		}
		if frontend == "" {
			frontend = pkg.JoinURLStrings(c.ClusterURL, "/oauth2/auth")
		}

		conf := oauth2.Config{
			ClientID:     clientId,
			ClientSecret: clientSecret,
			Endpoint: oauth2.Endpoint{
				TokenURL: backend,
				AuthURL:  frontend,
			},
			RedirectURL: redirectUrl,
			Scopes:      scopes,
		}

		state, err := sequence.RuneSequence(24, sequence.AlphaLower)
		pkg.Must(err, "Could not generate random state: %s", err)

		nonce, err := sequence.RuneSequence(24, sequence.AlphaLower)
		pkg.Must(err, "Could not generate random state: %s", err)

		location := conf.AuthCodeURL(string(state)) + "&nonce=" + string(nonce)

		if ok, _ := cmd.Flags().GetBool("no-open"); !ok {
			webbrowser.Open(location)
		}

		fmt.Println("Setting up callback listener on http://localhost:4445/callback")
		fmt.Println("Press ctrl + c on Linux / Windows or cmd + c on OSX to end the process.")
		fmt.Printf("If your browser does not open automatically, navigate to:\n\n\t%s\n\n", location)

		r := httprouter.New()
		server := &http.Server{Addr: ":4445", Handler: r}
		r.GET("/callback", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
			if r.URL.Query().Get("error") != "" {
				message := fmt.Sprintf("Got error: %s", r.URL.Query().Get("error_description"))
				fmt.Println(message)

				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(message))
				return
			}

			if r.URL.Query().Get("state") != string(state) {
				message := fmt.Sprintf("States do not match. Expected %s, got %s", string(state), r.URL.Query().Get("state"))
				fmt.Println(message)

				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(message))
				return
			}

			code := r.URL.Query().Get("code")
			token, err := conf.Exchange(ctx, code)
			pkg.Must(err, "Could not exchange code for token: %s", err)

			fmt.Printf("Access Token:\n\t%s\n", token.AccessToken)
			fmt.Printf("Refresh Token:\n\t%s\n\n", token.RefreshToken)
			fmt.Printf("Expires in:\n\t%s\n\n", token.Expiry)

			w.Write([]byte(fmt.Sprintf(`
<html><head></head><body>
<ul>
	<li>Access Token: <code>%s</code></li>
	<li>Refresh Token: <code>%s</code></li>
	<li>Expires in: <code>%s</code></li>
`, token.AccessToken, token.RefreshToken, token.Expiry)))

			idt := token.Extra("id_token")
			if idt != nil {
				w.Write([]byte(fmt.Sprintf(`<li>ID Token: <code>%s</code></li>`, idt)))
				fmt.Printf("ID Token:\n\t%s\n\n", idt)
			}
			w.Write([]byte("</ul></body></html>"))

			go func() {
				time.Sleep(time.Second * 1)
				ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
				server.Shutdown(ctx)
			}()
		})
		server.ListenAndServe()
	},
}

func init() {
	tokenCmd.AddCommand(tokenUserCmd)
	tokenUserCmd.Flags().Bool("no-open", false, "Do not open the browser window automatically")
	tokenUserCmd.Flags().StringSlice("scopes", []string{"hydra", "offline", "openid"}, "Force scopes")
	tokenUserCmd.Flags().String("id", "", "Force a client id, defaults to value from config file")
	tokenUserCmd.Flags().String("secret", "", "Force a client secret, defaults to value from config file")
	tokenUserCmd.Flags().String("redirect", "http://localhost:4445/callback", "Force a redirect url")
	tokenUserCmd.Flags().String("auth-url", c.ClusterURL, "Force the authorization url. The authorization url is the URL that the user will open in the browser, defaults to the cluster url value from config file")
	tokenUserCmd.Flags().String("token-url", c.ClusterURL, "Force a token url. The token url is used to exchange the auth code, defaults to the cluster url value from config file")
}
