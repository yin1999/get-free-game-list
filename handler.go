package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/db"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

const requestURL = "https://store-site-backend-static.ak.epicgames.com/freeGamesPromotions?locale=zh-CN&country=CN&allowCountries=CN"

func handler(w http.ResponseWriter, _ *http.Request) {
	var err error
	var resp string
	defer func() {
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		} else {
			os.Stderr.WriteString("task finished\n")
			w.Write([]byte(resp))
		}
	}()
	ctx, cc := context.WithTimeout(context.Background(), 45*time.Second)
	defer cc()
	var data gameList
	data, err = getFreeGameList(ctx, requestURL)
	if err != nil {
		return
	}
	data = data.FilterBy(available(time.Now()), // 当时有效
		after(time.Now().Add(-24*time.Hour)), // 当天生效
		discountLowerThan(10),                // 1折及以下
	)
	if len(data) == 0 {
		os.Stderr.WriteString("no new game avaliable\n")
		resp = "no new game avaliable\n"
		return
	}
	fmt.Fprintf(os.Stderr, "new game avaliable: %v\n", data)
	var client *firebase.App
	client, err = newClient(ctx)
	if err != nil {
		return
	}
	var messageClient *messaging.Client
	messageClient, err = client.Messaging(ctx)
	if err != nil {
		return
	}
	slugs := data.Slug()
	_, err = messageClient.Send(ctx, &messaging.Message{
		Notification: &messaging.Notification{
			Title: fmt.Sprintf("%d new game(s) avaliable", len(data)),
			Body:  fmt.Sprint(data),
		},
		Webpush: &messaging.WebpushConfig{
			FCMOptions: &messaging.WebpushFCMOptions{
				Link: "/epicfreegame?slug=" + url.QueryEscape(strings.Join(slugs, ";")),
			},
		},
		Topic: "all",
	})
	if err == nil {
		resp = "send notification to clients"
	}
	var dbClient *db.Client
	dbClient, err = client.Database(ctx)
	if err != nil {
		return
	}
	ref := dbClient.NewRef("freeGameList")
	err = ref.Set(ctx, slugs)
}

func newClient(ctx context.Context) (client *firebase.App, err error) {
	opt := option.WithCredentialsJSON([]byte(os.Getenv("firebaseadminsdk")))
	return firebase.NewApp(ctx, nil, opt)
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
	data := &struct {
		Data struct {
			Catalog struct {
				SearchStore struct {
					Elements gameList `json:"elements"`
				} `json:"searchStore"`
			} `json:"Catalog"`
		}
	}{}
	err = json.NewDecoder(res.Body).Decode(data)
	if err != nil {
		return nil, err
	}
	return data.Data.Catalog.SearchStore.Elements, err
}

type gameList []*gameData

type gameData struct {
	Title         string          `json:"title"`
	ProductSlug   string          `json:"productSlug"`
	OfferType     string          `json:"offerType"`
	UrlSlug       string          `json:"urlSlug"`
	CatalogNs     catalogN        `json:"catalogNs"`
	OfferMappings []pageMap       `json:"offerMappings"`
	Promotions    promotionStruct `json:"promotions"`
}

type catalogN struct {
	Mappings []pageMap `json:"mappings"`
}

type pageMap struct {
	PageSlug string `json:"pageSlug"`
	PageType string `json:"pageType"`
}

type promotionStruct struct {
	PromotionalOffers []promotions `json:"promotionalOffers"`
}

type promotions struct {
	PromotionalOffers []promotion `json:"promotionalOffers"`
}

type promotion struct {
	StartDate       time.Time `json:"startDate"`
	EndDate         time.Time `json:"endDate"`
	DiscountSetting discount  `json:"discountSetting"`
}

type discount struct {
	DiscountPercentage uint `json:"discountPercentage"`
}

type filterFunc func(g *gameData) bool

func available(t time.Time) filterFunc {
	return func(g *gameData) bool {
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
}

func after(t time.Time) filterFunc {
	return func(g *gameData) bool {
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
}

func discountLowerThan(discount uint) filterFunc {
	return func(g *gameData) bool {
		for i := range g.Promotions.PromotionalOffers {
			for j := range g.Promotions.PromotionalOffers[i].PromotionalOffers {
				t := g.Promotions.PromotionalOffers[i].PromotionalOffers[j]
				if t.DiscountSetting.DiscountPercentage <= discount {
					return true
				}
			}
		}
		return false
	}
}

func (data gameList) FilterBy(filters ...filterFunc) (res gameList) {
loop:
	for i := range data {
		for _, filter := range filters {
			if !filter(data[i]) {
				continue loop
			}
		}
		res = append(res, data[i])
	}
	return
}

func (data gameList) Slug() []string {
	res := make([]string, 0, len(data))
	for _, v := range data {
		slug := v.getPageSlug()
		res = append(res, finalSlug(slug, v.OfferType))
	}
	return res
}

func finalSlug(slug, offerType string) string {
	slug = "/" + slug
	switch offerType {
	case "BUNDLE":
		slug = "bundles" + slug
	case "OTHERS", "BASE_GAME", "ADD_ON":
		slug = "p" + slug
	default:
		slug = "p" + slug
		fmt.Fprintf(os.Stderr, "unknown offerType: %s\n", offerType)
	}
	return slug
}

func (v *gameData) catalog(key string) string {
	for i := range v.CatalogNs.Mappings {
		if v.CatalogNs.Mappings[i].PageType == key {
			return v.CatalogNs.Mappings[i].PageSlug
		}
	}
	return ""
}

func (v *gameData) offerMap(key string) string {
	for i := range v.OfferMappings {
		if v.OfferMappings[i].PageType == key {
			return v.OfferMappings[i].PageSlug
		}
	}
	return ""
}

func (v *gameData) getPageSlug() (slug string) {
	if v.OfferType == "ADD_ON" {
		return v.offerMap("offer")
	}
	if tmp := v.catalog("productHome"); tmp != "" {
		slug = tmp
	} else if v.ProductSlug != "" {
		slug = v.ProductSlug
	} else {
		slug = v.UrlSlug
	}
	if index := strings.IndexByte(slug, '/'); index != -1 {
		slug = slug[:index]
	}
	return
}

func (data gameData) String() string {
	return data.Title
}
