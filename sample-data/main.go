package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/gosimple/slug"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type server struct{}

type SampleData struct {
	Categories []SampleCategory
	Products   []SampleProduct
}

type SampleCategory struct {
	Id     int    `json:"id"`
	Name   string `json:"name"`
	Parent int    `json:"parent"`
}

type SampleProduct struct {
	Id       int     `json:"id"`
	Name     string  `json:"name"`
	Quantity int     `json:"quantity"`
	Value    float64 `json:"value"`
	Category int     `json:"category"`
	Parent   *SampleProduct
}

type FlatCategory struct {
	Name      string
	Slug      string
	Parent    string
	Ancestors []string
	Childrens []string
}

type MongoCategory struct {
	ID          primitive.ObjectID     `bson:"_id,omitempty"`
	Name        string                 `bson:"name,omitempty"`
	Slug        string                 `bson:"slug,omitempty"`
	Ancestors   []primitive.ObjectID   `bson:"ancestors,omitempty"`
	Childrens   []primitive.ObjectID   `bson:"childrens,omitempty"`
	LastUpdated *timestamppb.Timestamp `bson:"last_updated,omitempty"`
}

type FlatProduct struct {
	Name        string
	Slug        string
	Image       string
	Quantity    int
	Value       float64
	Category    primitive.ObjectID
	LastUpdated *timestamppb.Timestamp
}

var products, categories *mongo.Collection
var mongoCtx context.Context

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatalf("Error loading .env file")
	}
	mongoUsername := os.Getenv("MONGO_USERNAME")
	mongoPassword := os.Getenv("MONGO_PASSWORD")
	mongoDb := os.Getenv("MONGO_DB")

	mongoCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoUri := fmt.Sprintf("mongodb://%s:%s@localhost:27017", mongoUsername, mongoPassword)
	fmt.Println("Connecting to MongoDB...")
	client, err := mongo.Connect(mongoCtx, options.Client().ApplyURI(mongoUri))
	if err != nil {
		log.Fatalf("Error Starting MongoDB Client: %v", err)
	}

	products = client.Database(mongoDb).Collection("products")
	categories = client.Database(mongoDb).Collection("categories")

	sampleDataHandler()
}

func sampleDataHandler() {
	sd := getSampleData()
	cs := getFlatCategories(sd.Categories)
	mcs := handleCategories(cs)
	ps := getFlatProducts(sd, mcs)
	for _, p := range ps {
		insertProduct(p)
	}
}

func getSampleData() *SampleData {
	f, err := ioutil.ReadFile("./sample-data.json")
	if err != nil {
		fmt.Print(err)
	}
	sd := &SampleData{}
	_ = json.Unmarshal(f, sd)
	return sd
}

func getFlatCategories(s []SampleCategory) []*FlatCategory {
	cs := []*FlatCategory{}
	for _, c := range s {
		fc := &FlatCategory{
			Name: c.Name,
			Slug: slug.Make(c.Name),
		}
		for _, s := range s {
			if s.Id == c.Parent {
				fc.Parent = s.Name
			}
		}
		cs = append(cs, fc)
	}
	return cs
}

func handleCategories(cs []*FlatCategory) []*MongoCategory {
	mcs := []*MongoCategory{}
	for _, c := range cs {
		findChildrens(c, cs)
		findParent(c, cs)
		findAncestors(c, cs)
		id := insertCategory(c)
		mcs = append(mcs, &MongoCategory{
			ID:          id,
			Name:        c.Name,
			Slug:        c.Slug,
			LastUpdated: timestamppb.Now(),
		})
	}
	for _, mc := range mcs {
		for _, c := range cs {
			if c.Name == mc.Name {
				if len(c.Ancestors) != 0 {
					as := []primitive.ObjectID{}
					for _, ca := range c.Ancestors {
						a := findMongoCat(ca, mcs)
						as = append(as, a.ID)
					}
					mc.Ancestors = as
				}
				if len(c.Childrens) != 0 {
					chs := []primitive.ObjectID{}
					for _, cc := range c.Childrens {
						c := findMongoCat(cc, mcs)
						chs = append(chs, c.ID)
					}
					mc.Childrens = chs
				}
			}
		}
		updateCategory(mc)
	}
	return mcs
}

func findChildrens(c *FlatCategory, cs []*FlatCategory) {
	chs := []string{}
	for _, ch := range cs {
		if ch.Parent == c.Name {
			chs = append(chs, ch.Name)
			findChildrens(ch, cs)
		}
	}
	c.Childrens = chs
}

func findParent(c *FlatCategory, cs []*FlatCategory) {
	for _, ch := range c.Childrens {
		for _, p := range cs {
			if ch == p.Name {
				p.Parent = c.Name
			}
		}
	}
}

func findAncestors(c *FlatCategory, cs []*FlatCategory) {
	a := []string{}
	if c.Parent != "" {
		a = append(a, c.Parent)
	}
	for _, ch := range c.Childrens {
		for _, ca := range cs {
			if ch == ca.Name {
				a = append(a, ca.Parent)
				ca.Ancestors = dedupeString(a)
			}
		}
	}
}

func dedupeString(e []string) []string {
	m := map[string]bool{}
	r := []string{}
	for v := range e {
		if m[e[v]] == false {
			m[e[v]] = true
			r = append(r, e[v])
		}
	}
	return r
}

func insertCategory(c *FlatCategory) primitive.ObjectID {
	r, err := categories.InsertOne(mongoCtx, c)
	if err != nil {
		fmt.Println("InsertOne ERROR:", err)
	}
	return r.InsertedID.(primitive.ObjectID)
}

func findMongoCat(n string, mcs []*MongoCategory) *MongoCategory {
	mc := &MongoCategory{}
	for _, m := range mcs {
		if m.Name == n {
			mc = m
		}
	}
	return mc
}

func updateCategory(c *MongoCategory) {
	_, err := categories.ReplaceOne(mongoCtx, primitive.M{"_id": c.ID}, MongoCategory{
		Name:        c.Name,
		Slug:        c.Slug,
		Ancestors:   c.Ancestors,
		Childrens:   c.Childrens,
		LastUpdated: c.LastUpdated,
	})
	if err != nil {
		fmt.Printf("Cannot update person: %v", err)
	}
}

func getFlatProducts(s *SampleData, m []*MongoCategory) []*FlatProduct {
	ps := []*FlatProduct{}
	for _, p := range s.Products {
		fp := &FlatProduct{
			Name:     p.Name,
			Slug:     slug.Make(p.Name),
			Image:    strconv.Itoa(p.Id) + ".jpg",
			Quantity: p.Quantity,
			Value:    math.Ceil(p.Value*100) / 100,
		}
		var pc string
		for _, sc := range s.Categories {
			if sc.Id == p.Category {
				pc = sc.Name
			}
		}
		for _, c := range m {
			if c.Name == pc {
				fp.Category = c.ID
			}
		}
		ps = append(ps, fp)
	}
	return ps
}

func insertProduct(p *FlatProduct) {
	p.LastUpdated = timestamppb.Now()
	_, err := products.InsertOne(mongoCtx, p)
	if err != nil {
		fmt.Println("InsertOne ERROR:", err)
	}
}
