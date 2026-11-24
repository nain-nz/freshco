package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"smartfood/internal/service"
)

type AlertHandler struct{ svc *service.AlertService }

func NewAlertHandler(svc *service.AlertService) *AlertHandler { return &AlertHandler{svc: svc} }

// GET /api/alerts?unread=true&limit=50
func (h *AlertHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	data, err := h.svc.List(c.GetInt("product_id"), limit, c.Query("unread") == "true")
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// GET /api/alerts/unread-count  (badge lonceng di Dashboard)
func (h *AlertHandler) UnreadCount(c *gin.Context) {
	n, err := h.svc.UnreadCount(c.GetInt("product_id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": n})
}

// PATCH /api/alerts/:id/ack
func (h *AlertHandler) Ack(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		fail(c, service.NewValidationError("id alert tidak valid"))
		return
	}
	if err := h.svc.Acknowledge(c.GetInt("product_id"), id); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "alert ditandai sudah dibaca"})
}

// POST /api/products/:id/alerts/delete  body: {"ids":[1,2,3]}
// Memakai POST (bukan DELETE + body) supaya aman lewat proxy yang membuang body DELETE.
func (h *AlertHandler) Delete(c *gin.Context) {
	var body struct {
		IDs []int `json:"ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, service.NewValidationError("data tidak valid"))
		return
	}
	n, err := h.svc.DeleteMany(c.GetInt("product_id"), body.IDs)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "alert dihapus", "deleted": n})
}

// POST /api/alerts/ack-all
func (h *AlertHandler) AckAll(c *gin.Context) {
	if err := h.svc.AcknowledgeAll(c.GetInt("product_id")); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "semua alert ditandai sudah dibaca"})
}
