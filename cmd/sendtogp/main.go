package main

import (
	"bitbucket.org/rj/goey"
	"bitbucket.org/rj/goey/base"
	"bitbucket.org/rj/goey/loop"
	"bitbucket.org/rj/goey/windows"
	"context"
	"fmt"
	"github.com/99designs/keyring"
	"github.com/google/uuid"
	"github.com/gphotosuploader/google-photos-api-client-go/v2"
	"github.com/pkg/browser"
	"github.com/sqweek/dialog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/sys/unix"
	"log"
	"os"
	"strings"
)

const (
	service               = "send-to-google-photos"
	oauthTokenJsonFileKey = "oauth-token"
	oauth2JsonFileKey     = "GoogleOauth2JsonFile"
)

var (
	progressControl = &goey.Progress{
		Value: 0,
		Min:   0,
		Max:   0,
	}
	filesStr      = strings.Join(os.Args[1:], "\n")
	uploadContext context.Context
	stateUUID     = uuid.New().String()
)

func main() {
	err := loop.Run(createWindow)
	if err != nil {
		fmt.Println("Error: ", err)
	}
}

func createWindow() error {
	w, err := windows.NewWindow("Send To Google Photos", renderWindow())
	if err != nil {
		return err
	}
	w.SetScroll(false, true)
	return nil
}

func renderWindow() base.Widget {
	log.Print("Render")
	tabs := &goey.Tabs{
		Insets: goey.DefaultInsets(),
		Children: []goey.TabItem{
			renderUploadTab(),
			renderConfigTab(),
		},
	}
	return &goey.Padding{
		Insets: goey.DefaultInsets(),
		Child:  tabs,
	}
}

func renderConfigTab() goey.TabItem {
	var oauth2Json string
	return goey.TabItem{
		Caption: "Configuration / Authentication",
		Child: &goey.VBox{
			Children: []base.Widget{
				&goey.Label{Text: "Google GCP OAuth2 JSON file:"},
				&goey.TextArea{
					Value:       "",
					Placeholder: "Hidden",
					OnChange: func(v string) {
						oauth2Json = v
					},
				},
				&goey.Label{Text: "OAuth2 Token Exchange URL:"},
				&goey.TextArea{
					Value:       "",
					Placeholder: "Hidden",
					OnChange: func(v string) {
						oauth2Json = v
					},
				},
				&goey.HBox{Children: []base.Widget{
					&goey.Button{Text: "Set oauth keys", Default: true, OnClick: func() {
						setupOauthCreds(oauth2Json)
					}},
					&goey.Button{Text: "OAuth2 Authentication Part 1", Default: true, OnClick: func() {
						getOauth2TokenPart1()
					}},
					&goey.Button{Text: "OAuth2 Authentication Part 2", Default: true, OnClick: func() {
						getOauth2TokenPart2()
					}},
					&goey.Button{Text: "Test", Default: true, OnClick: func() {
						testCreds()
					}},
				}},
			},
		},
	}
}

func getOauth2TokenPart2() {

}

func getOauth2TokenPart1() {
	ring, _ := keyring.Open(keyring.Config{
		ServiceName: service,
	})

	oauth2ConfigJsonFile, err := ring.Get(oauth2JsonFileKey)
	if err != nil {
		log.Printf("Error: %s", err)
		dialog.Message("%s: %s", "Key chain error", err).Title("Key chain error").Error()
		return
	}

	c, err := google.ConfigFromJSON(oauth2ConfigJsonFile.Data)
	if err != nil {
		log.Printf("Error: %s", err)
		dialog.Message("%s: %s", "Oauth2 json file error", err).Title("Oauth2 json file error").Error()
		return
	}
	authUrl := c.AuthCodeURL(stateUUID)

	if err := browser.OpenURL(authUrl); err != nil {
		log.Printf("Error: %s", err)
		dialog.Message("%s: %s", "Browser open error", err).Title("Browser open error").Error()
		return
	}

	dialog.Message("%s", "Please authorize then copy paste the URL back here in the 'OAuth2 Token Exchange URL' field").Title("Oauth2 process").Info()

}

func setupOauthCreds(oauth2Json string) {
	ring, _ := keyring.Open(keyring.Config{
		ServiceName: service,
	})

	if err := AddSecret(ring, oauth2JsonFileKey, "OAuth2 JSON file", "OAuth2 JSON file", []byte(oauth2Json)); err != nil {
		log.Printf("Error: %s", err)
		dialog.Message("%s: %s", "Upload error", err).Title("Upload error").Error()
		return
	}
}

func AddSecret(ring keyring.Keyring, key string, label string, description string, data []byte) error {
	err := ring.Set(keyring.Item{
		Key:         key,
		Label:       label,
		Description: description,
		Data:        data,
	})
	if err != nil {
		return err
	}
	return nil
}

func testCreds() {
	ctx, cf := context.WithCancel(context.Background())
	defer func() {
		if cf != nil {
			cf()
			cf = nil
		}
	}()

	ring, _ := keyring.Open(keyring.Config{
		ServiceName: service,
	})

	secretOauthToken, err := ring.Get(oauthTokenJsonFileKey)
	if err != nil {
		log.Fatal(err)
	}

	ts, err := google.JWTConfigFromJSON(secretOauthToken.Data)
	if err != nil {
		log.Printf("JWT Token error: %s", err)
		dialog.Message("%s: %s", "JWT Token error", err).Title("Upload error").Error()
		return
	}

	c := oauth2.NewClient(ctx, ts.TokenSource(ctx))

	photosClient, err := gphotos.NewClient(c)
	if err != nil {
		log.Fatal(err)
	}
	albums, err := photosClient.Albums.List(ctx)
	if err != nil {
		log.Printf("List error: %s", err)
		dialog.Message("%s: %s", "list error", err).Title("List error").Error()
		return
	}

	for _, album := range albums {
		log.Printf("%s", album.Title)
	}
}

func renderUploadTab() goey.TabItem {
	return goey.TabItem{
		Caption: "Upload",
		Child: &goey.VBox{
			Children: []base.Widget{
				&goey.Label{Text: "Files:"},
				&goey.Expand{Child: &goey.TextArea{
					Value:       filesStr,
					Placeholder: "Files, one per line, full path.",
					OnChange:    func(v string) { println("Files ", v); filesStr = v },
					ReadOnly:    false,
				}},
				&goey.HBox{Children: []base.Widget{
					&goey.Button{Text: "Upload", Default: true, OnClick: func() {
						upload(false)
					}},
					&goey.Button{Text: "Upload then Delete", OnClick: func() {
						upload(true)
					}},
				}},
				progressControl,
			},
		},
	}
}

func upload(delete bool) {
	if uploadContext != nil {
		log.Print("Upload already in progress")
		dialog.Message("%s", "Upload already in progress").Title("Upload already in progress").Error()
		return
	}
	var cf func()
	uploadContext, cf = context.WithCancel(context.Background())
	defer func() {
		if cf != nil {
			cf()
			cf = nil
		}
		uploadContext = nil
	}()

	ring, _ := keyring.Open(keyring.Config{
		ServiceName: service,
	})

	secretOauthToken, err := ring.Get(oauthTokenJsonFileKey)
	if err != nil {
		log.Fatal(err)
	}

	ts, err := google.JWTConfigFromJSON(secretOauthToken.Data)
	if err != nil {
		log.Printf("JWT Token error: %s", err)
		dialog.Message("%s: %s", "JWT Token error", err).Title("Upload error").Error()
		return
	}

	c := oauth2.NewClient(uploadContext, ts.TokenSource(uploadContext))

	photosClient, err := gphotos.NewClient(c)
	if err != nil {
		log.Fatal(err)
	}

	files := strings.Split(filesStr, "\n")
	for i, file := range files {
		log.Print("Upload", i, "/", len(files), file)
		_, err = photosClient.Uploader.UploadFile(uploadContext, file)
		if err != nil {
			log.Printf("Error: %s, %s", file, err)
			dialog.Message("%s: %s: %s", "Upload error of", file, err).Title("Upload error").Error()
			break
		}
		if delete {
			log.Printf("Deleting %d: %s", i, file)
			if err := unix.Unlink(file); err != nil {
				log.Printf("Error deleting: %s: %s", file, err)
			}
		}
	}
}
