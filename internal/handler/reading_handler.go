package handler

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"smartfood/internal/model"
	"smartfood/internal/service"
)

type ReadingHandler struct{ svc *service.ReadingService }

func NewReadingHandler(svc *service.ReadingService) *ReadingHandler {
	return &ReadingHandler{svc: svc}
}

func parseFilter(c *gin.Context) (model.ReadingFilter, error) {
	f := model.ReadingFilter{ProductID: c.GetInt("product_id"), Channel: c.Query("channel")}
	var err error
	if s := c.Query("from"); s != "" {
		if f.From, err = parseTime(s, false); err != nil {
			return f, err
		}
	}
	if s := c.Query("to"); s != "" {
		if f.To, err = parseTime(s, true); err != nil {
			return f, err
		}
	}
	for _, name := range strings.Split(c.Query("channels"), ",") {
		if name = strings.TrimSpace(name); name != "" {
			f.Channels = append(f.Channels, name)
		}
	}
	f.Limit, _ = strconv.Atoi(c.Query("limit"))
	f.Offset, _ = strconv.Atoi(c.Query("offset"))
	return f, nil
}

// GET /api/channels
func (h *ReadingHandler) Channels(c *gin.Context) {
	chs, err := h.svc.Channels()
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": chs})
}

// GET /api/latest
func (h *ReadingHandler) Latest(c *gin.Context) {
	data, err := h.svc.Latest(c.GetInt("product_id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// GET /api/readings?channel=&from=&to=&limit=&offset=
func (h *ReadingHandler) History(c *gin.Context) {
	f, err := parseFilter(c)
	if err != nil {
		fail(c, err)
		return
	}
	res, err := h.svc.History(f)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// GET /api/readings/export?channel=&from=&to=  -> file CSV
func (h *ReadingHandler) Export(c *gin.Context) {
	f, err := parseFilter(c)
	if err != nil {
		fail(c, err)
		return
	}
	rows, err := h.svc.Export(f)
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="riwayat.csv"`)
	if err := service.WriteCSV(c.Writer, rows); err != nil {
		log.Printf("[export] gagal menulis csv: %v", err)
	}
}

// GET /api/stats?channel=&interval=5min|15min|30min|hour|6hour|day&from=&to=
func (h *ReadingHandler) Stats(c *gin.Context) {
	f, err := parseFilter(c)
	if err != nil {
		fail(c, err)
		return
	}
	res, err := h.svc.Stats(f.ProductID, f.Channel, c.Query("interval"), f.From, f.To)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// GET /api/products/:pid/readings/table?channels=suhu_atas,gas&from=&to=&limit=&offset=
// Tabel Riwayat: satu baris per waktu, sensor sebagai kolom. Sel yang tidak berubah
// diisi nilai terakhir yang diketahui dan ditandai changed = false.
func (h *ReadingHandler) Table(c *gin.Context) {
	f, err := parseFilter(c)
	if err != nil {
		fail(c, err)
		return
	}
	tf := model.TableFilter{
		ProductID: f.ProductID,
		Channels:  f.Channels,
		From:      f.From,
		To:        f.To,
		Limit:     f.Limit,
		Offset:    f.Offset,
	}
	res, err := h.svc.Table(tf)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
