package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	. "github.com/gugazimmermann/go-grpc-ecomm-go/ecommpb/ecommpb"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	. "go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
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
