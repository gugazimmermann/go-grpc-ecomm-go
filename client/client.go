package main

import (
	"context"
	"fmt"
	"log"

	. "github.com/gugazimmermann/go-grpc-ecomm-go/ecommpb/ecommpb"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func main() {
	fmt.Println("Starting Client...")
	cc, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Could not connect: %v", err)
	}
	defer cc.Close()
	cl := NewEcommServiceClient(cc)
	categoriesMenu(cl)
	CategoryBreadcrumb(cl)
	CategoriesSideMenu(cl)
}

func categoriesMenu(cl EcommServiceClient) {
	fmt.Println("Reading CategoriesMenu")
	res, err := cl.CategoriesMenu(context.Background(), &emptypb.Empty{})
	if err != nil {
		fmt.Printf("Error while reading the categories menu: %v\n", err)
	}
	fmt.Printf("Categories: %v\n", res)
}

func CategoryBreadcrumb(cl EcommServiceClient) {
	// Use a valid category SLUG
	slug := "world-of-darkness"
	fmt.Printf("Reading CategoryBreadcrumb with slug: %v\n", slug)
	res, err := cl.CategoryBreadcrumb(context.Background(), &CategoryRequest{Slug: slug})
	if err != nil {
		fmt.Printf("Error while reading the categories breadcrumb: %v\n", err)
	}
	fmt.Printf("Categories: %v\n", res)
}

func CategoriesSideMenu(cl EcommServiceClient) {
	// Use a valid category SLUG
	slug := "world-of-darkness"
	fmt.Printf("Reading CategoriesSideMenu with slug: %v\n", slug)
	res, err := cl.CategoriesSideMenu(context.Background(), &CategoryRequest{Slug: slug})
	if err != nil {
		fmt.Printf("Error while reading the categories sidemenu: %v\n", err)
	}
	fmt.Printf("Categories: %v\n", res)
}
