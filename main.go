package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	. "github.com/gugazimmermann/go-grpc-ecomm-go/ecommpb/ecommpb"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	. "go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type server struct{}

type MongoCategories struct {
	ID            ObjectID               `bson:"_id,omitempty"`
	Name          string                 `bson:"name,omitempty"`
	Slug          string                 `bson:"slug,omitempty"`
	Subcategories []*MongoCategories     `bson:"subcategories,omitempty"`
	Parents       []*MongoCategories     `bson:"parents,omitempty"`
	LastUpdated   *timestamppb.Timestamp `bson:"last_updated,omitempty"`
}

type MongoProducts struct {
	Metadata []MongoProductsMetadata `bson:"metadata,omitempty"`
	Data     []MongoProductsData     `bson:"data,omitempty"`
}

type MongoProductsMetadata struct {
	Total int32 `bson:"total,omitempty"`
}

type MongoProductsData struct {
	ID          ObjectID               `bson:"_id,omitempty"`
	Name        string                 `bson:"name,omitempty"`
	Slug        string                 `bson:"slug,omitempty"`
	Image       string                 `bson:"image,omitempty"`
	Quantity    int32                  `bson:"quantity,omitempty"`
	Value       float64                `bson:"value,omitempty"`
	Category    ObjectID               `bson:"category,omitempty"`
	Cat         []MongoCategories      `bson:"cat,omitempty"`
	LastUpdated *timestamppb.Timestamp `bson:"lastupdated,omitempty"`
}

type Body struct {
	Sub               string `json:"sub,omitempty"`
	EmailVerified     bool   `json:"email_verified,omitempty"`
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	GivenName         string `json:"given_name,omitempty"`
	FamilyName        string `json:"family_name,omitempty"`
	Email             string `json:"email,omitempty"`
}

var products, categories *mongo.Collection

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	err := godotenv.Load(".env")
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

	fmt.Println("Starting Listener...")
	l, err := net.Listen("tcp", "0.0.0.0:50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	opts := []grpc.ServerOption{}
	s := grpc.NewServer(opts...)
	RegisterEcommServiceServer(s, &server{})

	go func() {
		fmt.Println("Ecomm Server Started...")
		if err := s.Serve(l); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)

	<-ch
	fmt.Println("Stopping Ecomm Server...")
	s.Stop()
	fmt.Println("Closing Listener...")
	l.Close()
	fmt.Println("Closing MongoDB...")
	client.Disconnect(mongoCtx)
	fmt.Println("All done!")
}

func (*server) CategoriesMenu(ctx context.Context, req *emptypb.Empty) (*CategoriesMenuResponse, error) {
	log.Println("CategoriesMenu called")
	matchStage := bson.D{E{Key: "$match", Value: bson.D{
		E{Key: "ancestors", Value: nil},
	}}}
	graphLookupStage := bson.D{
		E{Key: "$graphLookup", Value: bson.D{
			E{Key: "from", Value: "categories"},
			E{Key: "startWith", Value: "$childrens"},
			E{Key: "connectFromField", Value: "childrens"},
			E{Key: "connectToField", Value: "_id"},
			E{Key: "maxDepth", Value: 0},
			E{Key: "as", Value: "subcategories"},
		}}}
	cur, err := categories.Aggregate(context.Background(), mongo.Pipeline{matchStage, graphLookupStage})
	if err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Unknown Internal Error: %v", err))
	}
	defer cur.Close(context.Background())
	ds := []*MongoCategories{}
	for cur.Next(context.Background()) {
		d := &MongoCategories{}
		if err := cur.Decode(d); err != nil {
			return nil, status.Errorf(codes.Internal, fmt.Sprintf("Cannot decoding data: %v", err))
		}
		ds = append(ds, d)
	}
	if err = cur.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Unknown Internal Error: %v", err))
	}

	res := []*Category{}
	for _, d := range ds {
		ccs := []*Category{}
		if len(d.Subcategories) > 0 {
			for _, cc := range d.Subcategories {
				ec := &Category{
					Id:   cc.ID.Hex(),
					Name: cc.Name,
					Slug: cc.Slug,
				}
				ccs = append(ccs, ec)
			}
		}
		r := &Category{
			Id:          d.ID.Hex(),
			Name:        d.Name,
			Slug:        d.Slug,
			Childrens:   ccs,
			LastUpdated: d.LastUpdated,
		}
		res = append(res, r)
	}
	return &CategoriesMenuResponse{
		Categories: res,
	}, nil
}

func (*server) CategoryBreadcrumb(ctx context.Context, req *CategoryRequest) (*CategoriesMenuResponse, error) {
	s := req.GetSlug()
	log.Printf("CategoryBreadcrumb called with slug: %v\n", s)
	matchStage := bson.D{E{Key: "$match", Value: bson.D{
		E{Key: "slug", Value: s},
	}}}
	graphLookupStage := bson.D{
		E{Key: "$graphLookup", Value: bson.D{
			E{Key: "from", Value: "categories"},
			E{Key: "startWith", Value: "$ancestors"},
			E{Key: "connectFromField", Value: "ancestors"},
			E{Key: "connectToField", Value: "_id"},
			E{Key: "maxDepth", Value: 0},
			E{Key: "as", Value: "parents"},
		}}}
	cur, err := categories.Aggregate(context.Background(), mongo.Pipeline{matchStage, graphLookupStage})
	if err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Unknown Internal Error: %v", err))
	}
	defer cur.Close(context.Background())
	ds := []*MongoCategories{}
	for cur.Next(context.Background()) {
		d := &MongoCategories{}
		if err := cur.Decode(d); err != nil {
			return nil, status.Errorf(codes.Internal, fmt.Sprintf("Cannot decoding data: %v", err))
		}
		ds = append(ds, d)
	}
	if err = cur.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Unknown Internal Error: %v", err))
	}

	res := []*Category{}
	for _, d := range ds {
		cps := []*Category{}
		if len(d.Parents) > 0 {
			for _, cp := range d.Parents {
				ec := &Category{
					Id:   cp.ID.Hex(),
					Name: cp.Name,
					Slug: cp.Slug,
				}
				cps = append(cps, ec)
			}
		}
		r := &Category{
			Id:          d.ID.Hex(),
			Name:        d.Name,
			Slug:        d.Slug,
			Ancestors:   cps,
			LastUpdated: d.LastUpdated,
		}
		res = append(res, r)
	}
	return &CategoriesMenuResponse{
		Categories: res,
	}, nil
}

func (*server) CategoriesSideMenu(ctx context.Context, req *CategoryRequest) (*CategoriesMenuResponse, error) {
	s := req.GetSlug()
	log.Printf("CategoriesSideMenu called with slug: %v\n", s)
	matchStage := bson.D{E{Key: "$match", Value: bson.D{
		E{Key: "slug", Value: s},
	}}}
	graphLookupStage := bson.D{
		E{Key: "$graphLookup", Value: bson.D{
			E{Key: "from", Value: "categories"},
			E{Key: "startWith", Value: "$childrens"},
			E{Key: "connectFromField", Value: "childrens"},
			E{Key: "connectToField", Value: "_id"},
			E{Key: "maxDepth", Value: 0},
			E{Key: "as", Value: "subcategories"},
		}}}
	cur, err := categories.Aggregate(context.Background(), mongo.Pipeline{matchStage, graphLookupStage})
	if err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Unknown Internal Error: %v", err))
	}
	defer cur.Close(context.Background())
	ds := []*MongoCategories{}
	for cur.Next(context.Background()) {
		d := &MongoCategories{}
		if err := cur.Decode(d); err != nil {
			return nil, status.Errorf(codes.Internal, fmt.Sprintf("Cannot decoding data: %v", err))
		}
		ds = append(ds, d)
	}
	if err = cur.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Unknown Internal Error: %v", err))
	}

	res := []*Category{}
	for _, d := range ds {
		ccs := []*Category{}
		if len(d.Subcategories) > 0 {
			for _, cc := range d.Subcategories {
				ec := &Category{
					Id:   cc.ID.Hex(),
					Name: cc.Name,
					Slug: cc.Slug,
				}
				ccs = append(ccs, ec)
			}
		}
		r := &Category{
			Id:          d.ID.Hex(),
			Name:        d.Name,
			Slug:        d.Slug,
			Childrens:   ccs,
			LastUpdated: d.LastUpdated,
		}
		res = append(res, r)
	}
	return &CategoriesMenuResponse{
		Categories: res,
	}, nil
}

func dataToProd(p MongoProductsData) *Product {
	return &Product{
		Id:       p.ID.Hex(),
		Name:     p.Name,
		Slug:     p.Slug,
		Image:    p.Image,
		Quantity: p.Quantity,
		Value:    float32(math.Ceil(p.Value*100) / 100),
		Category: &Category{
			Id:   p.Cat[0].ID.Hex(),
			Name: p.Cat[0].Name,
			Slug: p.Cat[0].Slug,
		},
		LastUpdated: p.LastUpdated,
	}
}

func (*server) Products(ctx context.Context, req *ProductRequest) (*ProductsResponse, error) {
	start := req.GetStart()
	qty := req.GetQty()
	log.Printf("Products called with start: %v | qty: %v\n", start, qty)
	sortStage := bson.D{E{Key: "$sort", Value: bson.D{E{Key: "name", Value: 1}}}}
	graphLookupStage := bson.D{
		E{Key: "$graphLookup", Value: bson.D{
			E{Key: "from", Value: "categories"},
			E{Key: "startWith", Value: "$category"},
			E{Key: "connectFromField", Value: "category"},
			E{Key: "connectToField", Value: "_id"},
			E{Key: "maxDepth", Value: 0},
			E{Key: "as", Value: "cat"},
		}}}
	facetStage := bson.D{
		E{Key: "$facet", Value: bson.D{
			E{Key: "metadata", Value: []bson.D{{E{Key: "$count", Value: "total"}}}},
			E{Key: "data", Value: []bson.D{{E{Key: "$skip", Value: start}}, {E{Key: "$limit", Value: qty}}}},
		}},
	}
	cur, err := products.Aggregate(context.Background(), mongo.Pipeline{sortStage, graphLookupStage, facetStage})
	if err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Unknown Internal Error: %v", err))
	}
	d := &MongoProducts{}
	defer cur.Close(context.Background())
	for cur.Next(context.Background()) {
		if err := cur.Decode(d); err != nil {
			return nil, status.Errorf(codes.Internal, fmt.Sprintf("Cannot decoding data: %v", err))
		}
	}
	if err = cur.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Unknown Internal Error: %v", err))
	}
	data := []*Product{}
	for _, p := range d.Data {
		data = append(data, dataToProd(p))
	}
	return &ProductsResponse{
		Total: d.Metadata[0].Total,
		Data:  data,
	}, nil
}

func seeProductCategories(oid ObjectID) []ObjectID {
	matchStage := bson.D{E{Key: "$match", Value: bson.D{
		E{Key: "_id", Value: oid},
	}}}
	graphLookupStage := bson.D{
		E{Key: "$graphLookup", Value: bson.D{
			E{Key: "from", Value: "categories"},
			E{Key: "startWith", Value: "$childrens"},
			E{Key: "connectFromField", Value: "childrens"},
			E{Key: "connectToField", Value: "_id"},
			E{Key: "as", Value: "subcategories"},
		}}}
	cur, err := categories.Aggregate(context.Background(), mongo.Pipeline{matchStage, graphLookupStage})
	if err != nil {
		fmt.Printf("Unknown Internal Error: %v", err)
	}
	defer cur.Close(context.Background())
	ds := []*MongoCategories{}
	for cur.Next(context.Background()) {
		d := &MongoCategories{}
		if err := cur.Decode(d); err != nil {
			fmt.Printf("Cannot decoding data: %v", err)
		}
		ds = append(ds, d)
	}
	if err = cur.Err(); err != nil {
		fmt.Printf("Unknown Internal Error: %v", err)
	}

	cats := []ObjectID{}
	if len(ds[0].Subcategories) > 0 {
		for _, cat := range ds[0].Subcategories {
			cats = append(cats, cat.ID)
		}
	}
	return cats
}

func (*server) ProductsFromCategory(ctx context.Context, req *ProductFromCategoryRequest) (*ProductsResponse, error) {
	categoryID := req.GetCategoryId()
	start := req.GetStart()
	qty := req.GetQty()
	log.Printf("ProductsFromCategory called with Category ID: %v | start: %v | qty: %v\n", categoryID, start, qty)
	oid, err := primitive.ObjectIDFromHex(categoryID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Cannot parse ID")
	}
	cats := seeProductCategories(oid)
	search := bson.D{}
	if len(cats) > 0 {
		arr := []bson.D{}
		for _, i := range cats {
			arr = append(arr, bson.D{E{Key: "category", Value: i}})
		}
		search = bson.D{E{Key: "$or", Value: arr}}
	} else {
		search = bson.D{E{Key: "category", Value: oid}}
	}
	matchStage := bson.D{E{Key: "$match", Value: search}}
	sortStage := bson.D{E{Key: "$sort", Value: bson.D{E{Key: "name", Value: 1}}}}
	graphLookupStage := bson.D{
		E{Key: "$graphLookup", Value: bson.D{
			E{Key: "from", Value: "categories"},
			E{Key: "startWith", Value: "$category"},
			E{Key: "connectFromField", Value: "category"},
			E{Key: "connectToField", Value: "_id"},
			E{Key: "maxDepth", Value: 0},
			E{Key: "as", Value: "cat"},
		}}}
	facetStage := bson.D{
		E{Key: "$facet", Value: bson.D{
			E{Key: "metadata", Value: []bson.D{{E{Key: "$count", Value: "total"}}}},
			E{Key: "data", Value: []bson.D{{E{Key: "$skip", Value: start}}, {E{Key: "$limit", Value: qty}}}},
		}},
	}
	cur, err := products.Aggregate(context.Background(), mongo.Pipeline{matchStage, sortStage, graphLookupStage, facetStage})
	if err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Unknown Internal Error: %v", err))
	}
	d := &MongoProducts{}
	defer cur.Close(context.Background())
	for cur.Next(context.Background()) {
		if err := cur.Decode(d); err != nil {
			return nil, status.Errorf(codes.Internal, fmt.Sprintf("Cannot decoding data: %v", err))
		}
	}
	if err = cur.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Unknown Internal Error: %v", err))
	}
	data := []*Product{}
	if len(d.Data) > 0 {
		for _, p := range d.Data {
			data = append(data, dataToProd(p))
		}
		return &ProductsResponse{Total: d.Metadata[0].Total, Data: data}, nil
	} else {
		return &ProductsResponse{Total: 0, Data: data}, nil
	}
}

func (*server) SearchProducts(ctx context.Context, req *SearchProductsRequest) (*ProductsResponse, error) {
	name := req.GetName()
	start := req.GetStart()
	qty := req.GetQty()
	log.Printf("SearchProducts called with Name: %v | start: %v | qty: %v\n", name, start, qty)
	matchStage := bson.D{E{Key: "$match", Value: bson.D{
		E{Key: "name", Value: Regex{Pattern: name, Options: "i"}},
	}}}
	sortStage := bson.D{E{Key: "$sort", Value: bson.D{E{Key: "name", Value: 1}}}}
	graphLookupStage := bson.D{
		E{Key: "$graphLookup", Value: bson.D{
			E{Key: "from", Value: "categories"},
			E{Key: "startWith", Value: "$category"},
			E{Key: "connectFromField", Value: "category"},
			E{Key: "connectToField", Value: "_id"},
			E{Key: "maxDepth", Value: 0},
			E{Key: "as", Value: "cat"},
		}}}
	facetStage := bson.D{
		E{Key: "$facet", Value: bson.D{
			E{Key: "metadata", Value: []bson.D{{E{Key: "$count", Value: "total"}}}},
			E{Key: "data", Value: []bson.D{{E{Key: "$skip", Value: start}}, {E{Key: "$limit", Value: qty}}}},
		}},
	}
	cur, err := products.Aggregate(context.Background(), mongo.Pipeline{matchStage, graphLookupStage, sortStage, facetStage})
	if err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Unknown Internal Error: %v", err))
	}
	d := &MongoProducts{}
	defer cur.Close(context.Background())
	for cur.Next(context.Background()) {
		if err := cur.Decode(d); err != nil {
			return nil, status.Errorf(codes.Internal, fmt.Sprintf("Cannot decoding data: %v", err))
		}
	}
	if err = cur.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Unknown Internal Error: %v", err))
	}
	data := []*Product{}
	if len(d.Data) > 0 {
		for _, p := range d.Data {
			data = append(data, dataToProd(p))
		}
		return &ProductsResponse{Total: d.Metadata[0].Total, Data: data}, nil
	} else {
		return &ProductsResponse{Total: 0, Data: data}, nil
	}
}

func (*server) Checkout(ctx context.Context, req *CheckoutRequest) (*wrapperspb.BoolValue, error) {
	log.Println("Checkout called")
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "Retrieving metadata is failed")
	}
	token := md["x-user-auth-token"]
	res, err := keycloak(token[0])
	if err != nil {
		return wrapperspb.Bool(false), nil
	}
	if res.Status == "401 Unauthorized" {
		log.Println("Checkout Unauthorized")
		return wrapperspb.Bool(false), nil
	}
	defer res.Body.Close()
	b := &Body{}
	if err := json.NewDecoder(res.Body).Decode(&b); err != nil {
		fmt.Println(err)
		return nil, status.Errorf(codes.Internal, "Error decoding keycloak response body")
	}
	fmt.Println(b)
	log.Printf("Checkout to: %v - %v\n", b.Name, b.Email)

	log.Printf("Products: %v", req.Cart)
	return wrapperspb.Bool(true), nil
}

func keycloak(token string) (*http.Response, error) {
	kcu := os.Getenv("KEYCLOAK_URL")
	fmt.Println(kcu)
	data := url.Values{}
	data.Set("access_token", token)
	res, err := http.Post(kcu, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("Keycloak Error: %v", err))
	}
	return res, nil
}
