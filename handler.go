package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/messaging"
	"google.golang.org/api/option"
)

const (
	requestURL = "https://store-site-backend-static.ak.epicgames.com/freeGamesPromotions?locale=zh-CN&country=CN&allowCountries=CN"
)

func handler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp string
	defer func() {
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		} else {
			log.Print("task finished\n")
			w.Write([]byte(resp))
		}
	}()
	ctx, cc := context.WithTimeout(context.Background(), 30*time.Second)
	defer cc()
	var data gameList
	data, err = getFreeGameList(ctx, requestURL)
	if err != nil {
		return
	}
	data = data.available(time.Now()).Near(24 * time.Hour)
	if len(data) == 0 {
		log.Print("no new game avaliable\n")
		resp = "no new game avaliable\n"
		return
	}
	log.Printf("new game avaliable: %v\n", data)
	var client *messaging.Client
	client, err = newClient()
	if err != nil {
		log.Println(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, err.Error())
		return
	}
	_, err = client.Send(ctx, &messaging.Message{
		Notification: &messaging.Notification{
			Title: fmt.Sprintf("%d new game avaliable", len(data)),
			Body:  fmt.Sprint(data),
		},
		Webpush: &messaging.WebpushConfig{
			FcmOptions: &messaging.WebpushFcmOptions{
				Link: "/epicfreegame?slug=" + data.Slug(),
			},
		},
		Topic: "all",
	})
	if err == nil {
		resp = "send notification to clients"
	}
}

func newClient() (client *messaging.Client, err error) {
	ctx := context.Background()
	opt := option.WithCredentialsJSON([]byte(os.Getenv("firebaseadminsdk")))
	var app *firebase.App
	app, err = firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return
	}
	client, err = app.Messaging(ctx)
	return
}

func getFreeGameList(ctx context.Context, url string) (gameList, error) {
	req, err := http.NewRequestWithContext(ctx,
		http.MethodGet,
		url,
		http.NoBody)
	if err != nil {
		return nil, err
	}
	var res *http.Response
	if res, err = http.DefaultClient.Do(req); err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data := &responseData{}
	err = json.NewDecoder(res.Body).Decode(&data)
	if err != nil {
		data = nil
	}
	return data.Data.Catalog.SearchStore.Elements, err
}

type responseData struct {
	Data dataStruct
}

type dataStruct struct {
	Catalog catalogStruct `json:"Catalog"`
}

type catalogStruct struct {
	SearchStore searchStoreStruct `json:"searchStore"`
}

type searchStoreStruct struct {
	Elements gameList `json:"elements"`
}

type gameList []gameData

type gameData struct {
	Title       string          `json:"title"`
	ID          string          `json:"id"`
	ProductSlug string          `json:"productSlug"`
	Promotions  promotionStruct `json:"promotions"`
}

type promotionStruct struct {
	PromotionalOffers []promotions `json:"promotionalOffers"`
}

type promotions struct {
	PromotionalOffers []promotion `json:"promotionalOffers"`
}

type promotion struct {
	StartDate time.Time `json:"startDate"`
	EndDate   time.Time `json:"endDate"`
}

func (g gameData) available(t time.Time) bool {
	var timeInfo promotion
	for i := range g.Promotions.PromotionalOffers {
		for j := range g.Promotions.PromotionalOffers[i].PromotionalOffers {
			timeInfo = g.Promotions.PromotionalOffers[i].PromotionalOffers[j]
			if timeInfo.StartDate.Before(t) && timeInfo.EndDate.After(t) {
				return true
			}
		}
	}
	return false
}

func (g gameData) after(t time.Time) bool {
	var timeInfo promotion
	for i := range g.Promotions.PromotionalOffers {
		for j := range g.Promotions.PromotionalOffers[i].PromotionalOffers {
			timeInfo = g.Promotions.PromotionalOffers[i].PromotionalOffers[j]
			if timeInfo.StartDate.After(t) {
				return true
			}
		}
	}
	return false
}

func (data gameList) available(t time.Time) (res gameList) {
	for i := range data {
		if data[i].available(t) {
			res = append(res, data[i])
		}
	}
	return
}

func (data gameList) Near(d time.Duration) (res gameList) {
	t := time.Now().Add(-d)
	for i := range data {
		if data[i].after(t) {
			res = append(res, data[i])
		}
	}
	return
}

func (data gameList) Slug() string {
	res := ""
	for i := range data {
		if i != 0 {
			res += ";"
		}
		res += data[i].ProductSlug
	}
	return res
}

func (data gameData) String() string {
	return data.Title
}
