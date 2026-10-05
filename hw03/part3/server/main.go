package main

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

type album struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Artist string  `json:"artist"`
	Price  float64 `json:"price"`
}

var (
	mu     sync.RWMutex
	albums = map[string]album{
		"1": {ID: "1", Title: "Blue Train", Artist: "John Coltrane", Price: 56.99},
		"2": {ID: "2", Title: "Jeru", Artist: "Gerry Mulligan", Price: 17.99},
		"3": {ID: "3", Title: "Sarah Vaughan and Clifford Brown", Artist: "Sarah Vaughan", Price: 39.99},
	}
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/albums", getAlbums)
	router.GET("/albums/:id", getAlbumByID)
	router.POST("/albums", postAlbums)
	router.Run(":8080")
}

func getAlbums(c *gin.Context) {
	mu.RLock()
	list := make([]album, 0, len(albums))
	for _, a := range albums {
		list = append(list, a)
	}
	mu.RUnlock()
	c.JSON(http.StatusOK, list)
}

func postAlbums(c *gin.Context) {
	var newAlbum album
	if err := c.BindJSON(&newAlbum); err != nil {
		return
	}
	mu.Lock()
	albums[newAlbum.ID] = newAlbum
	mu.Unlock()
	c.JSON(http.StatusCreated, newAlbum)
}

func getAlbumByID(c *gin.Context) {
	mu.RLock()
	a, ok := albums[c.Param("id")]
	mu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"message": "album not found"})
		return
	}
	c.JSON(http.StatusOK, a)
}
