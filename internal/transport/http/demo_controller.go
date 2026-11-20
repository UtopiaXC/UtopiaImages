package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/service"
	"github.com/gin-gonic/gin"
)

type DemoController struct {
	resetService *service.DemoResetService
}

func NewDemoController(rs *service.DemoResetService) *DemoController {
	return &DemoController{resetService: rs}
}

func (ctrl *DemoController) RegisterRoutes(rg *gin.RouterGroup) {
	demo := rg.Group("/demo")
	{
		demo.GET("/status", ctrl.GetStatus)
		demo.GET("/events", ctrl.StreamEvents)
	}
}

func (ctrl *DemoController) GetStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.JSON(http.StatusOK, ctrl.resetService.GetStatus())
}

func (ctrl *DemoController) StreamEvents(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	sub := ctrl.resetService.Subscribe()
	defer ctrl.resetService.Unsubscribe(sub)

	// Send initial status event immediately
	initialBytes, _ := json.Marshal(ctrl.resetService.GetStatus())
	_, _ = fmt.Fprintf(c.Writer, "event: status\ndata: %s\n\n", initialBytes)
	c.Writer.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	ctx := c.Request.Context()
	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctx.Done():
			return false
		case <-heartbeat.C:
			_, _ = fmt.Fprintf(w, ": ping\n\n")
			return true
		case status, ok := <-sub:
			if !ok {
				return false
			}
			data, _ := json.Marshal(status)
			_, _ = fmt.Fprintf(w, "event: reset\ndata: %s\n\n", data)
			return true
		}
	})
}
