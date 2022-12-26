package main

import (
	"bitbucket.org/rj/goey"
	"bitbucket.org/rj/goey/base"
	"bitbucket.org/rj/goey/dialog"
	"bitbucket.org/rj/goey/loop"
	"bitbucket.org/rj/goey/windows"
	"context"
	"fmt"
	"github.com/99designs/keyring"
	"github.com/google/uuid"
	"github.com/gphotosuploader/google-photos-api-client-go/v2"
	"github.com/pkg/browser"
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
		dialog.NewMessage(fmt.Sprintf("%s: %s", "Key chain error", err)).WithTitle("Key chain error").WithError().Show()
		return
	}

	c, err := google.ConfigFromJSON(oauth2ConfigJsonFile.Data)
	if err != nil {
		log.Printf("Error: %s", err)
		dialog.NewMessage(fmt.Sprintf("%s: %s", "Oauth2 json file error", err)).WithTitle("Oauth2 json file error").WithError().Show()
		return
	}
	authUrl := c.AuthCodeURL(stateUUID)

	if err := browser.OpenURL(authUrl); err != nil {
		log.Printf("Error: %s", err)
		dialog.NewMessage(fmt.Sprintf("%s: %s", "Browser open error", err)).WithTitle("Browser open error").WithError().Show()
		return
	}

	dialog.NewMessage(fmt.Sprintf("%s", "Please authorize then copy paste the URL back here in the 'OAuth2 Token Exchange URL' field")).WithTitle("Oauth2 process").WithInfo().Show()

}

func setupOauthCreds(oauth2Json string) {
	ring, _ := keyring.Open(keyring.Config{
		ServiceName: service,
	})

	if err := AddSecret(ring, oauth2JsonFileKey, "OAuth2 JSON file", "OAuth2 JSON file", []byte(oauth2Json)); err != nil {
		log.Printf("Error: %s", err)
		dialog.NewMessage(fmt.Sprintf("%s: %s", "Upload error", err)).WithTitle("Upload error").WithError().Show()
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
		log.Printf("Keyring error: %s", err)
		dialog.NewMessage(fmt.Sprintf("%s: %s", "Keyring error", err)).WithTitle("Keyring error").WithError().Show()
		return
	}

	ts, err := google.JWTConfigFromJSON(secretOauthToken.Data)
	if err != nil {
		log.Printf("JWT Token error: %s", err)
		dialog.NewMessage(fmt.Sprintf("%s: %s", "JWT Token error", err)).WithTitle("JWT Token error").WithError().Show()
		return
	}

	c := oauth2.NewClient(ctx, ts.TokenSource(ctx))

	photosClient, err := gphotos.NewClient(c)
	if err != nil {
		log.Printf("Photo client error: %s", err)
		dialog.NewMessage(fmt.Sprintf("%s: %s", "Photo client error", err)).WithTitle("Photo client error").WithError().Show()
		return
	}
	albums, err := photosClient.Albums.List(ctx)
	if err != nil {
		log.Printf("List error: %s", err)
		dialog.NewMessage(fmt.Sprintf("%s: %s", "list error", err)).WithTitle("List error").WithError().Show()
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
		dialog.NewMessage(fmt.Sprintf("%s", "Upload already in progress")).WithTitle("Upload already in progress").WithError().Show()
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
		log.Printf("Keyring error: %s", err)
		dialog.NewMessage(fmt.Sprintf("%s: %s", "Keyring error", err)).WithTitle("Keyring error").WithError().Show()
		return
	}

	ts, err := google.JWTConfigFromJSON(secretOauthToken.Data)
	if err != nil {
		log.Printf("JWT Token error: %s", err)
		dialog.NewMessage(fmt.Sprintf("%s: %s", "JWT Token error", err)).WithTitle("Upload error").WithError().Show()
		return
	}

	c := oauth2.NewClient(uploadContext, ts.TokenSource(uploadContext))

	photosClient, err := gphotos.NewClient(c)
	if err != nil {
		log.Printf("Photo client error: %s", err)
		dialog.NewMessage(fmt.Sprintf("%s: %s", "Photo client error", err)).WithTitle("Photo client error").WithError().Show()
		return
	}

	files := strings.Split(filesStr, "\n")
	for i, file := range files {
		log.Print("Upload", i, "/", len(files), file)
		_, err = photosClient.Uploader.UploadFile(uploadContext, file)
		if err != nil {
			log.Printf("Error: %s, %s", file, err)
			dialog.NewMessage(fmt.Sprintf("%s: %s: %s", "Upload error of", file, err)).WithTitle("Upload error").WithError().Show()
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
