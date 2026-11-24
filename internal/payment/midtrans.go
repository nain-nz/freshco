package payment

import (
	"bytes"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Midtrans struct {
	serverKey string
	sandbox   bool
	notifyURL string
	http      *http.Client
}

func (m *Midtrans) WithNotifyURL(base string) *Midtrans {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base != "" {
		m.notifyURL = base + "/api/payments/notify"
	}
	return m
}

// WebhookEnabled true kalau Midtrans akan memberi kabar sendiri (tidak perlu ditanya).
func (m *Midtrans) WebhookEnabled() bool { return m != nil && m.notifyURL != "" }

func NewMidtrans(serverKey string, sandbox bool) *Midtrans {
	return &Midtrans{serverKey: serverKey, sandbox: sandbox, http: &http.Client{Timeout: 15 * time.Second}}
}

// Enabled false kalau server key belum diisi.
func (m *Midtrans) Enabled() bool { return m != nil && m.serverKey != "" }

func (m *Midtrans) apiBase() string {
	if m.sandbox {
		return "https://api.sandbox.midtrans.com"
	}
	return "https://api.midtrans.com"
}

// Notification = isi notifikasi (webhook) dan respons status Midtrans yang kita pakai.
type Notification struct {
	OrderID           string `json:"order_id"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
}

// VerifySignature: signature_key = SHA512(order_id + status_code + gross_amount + ServerKey).
func (m *Midtrans) VerifySignature(n Notification) bool {
	if !m.Enabled() || n.SignatureKey == "" {
		return false
	}
	sum := sha512.Sum512([]byte(n.OrderID + n.StatusCode + n.GrossAmount + m.serverKey))
	want := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(want), []byte(strings.ToLower(n.SignatureKey))) == 1
}

func (m *Midtrans) do(req *http.Request) ([]byte, int, error) {
	req.SetBasicAuth(m.serverKey, "")
	req.Header.Set("Accept", "application/json")
	res, err := m.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return b, res.StatusCode, err
}

// QRIS = hasil pembuatan QR: teks QR untuk digambar aplikasi dan batas waktunya.
type QRIS struct {
	QRString  string
	ImageURL  string // URL gambar QR; simulator sandbox memintanya
	ExpiresAt time.Time
}

// Sandbox true kalau memakai server sandbox Midtrans.
func (m *Midtrans) Sandbox() bool { return m != nil && m.sandbox }

// wib: Midtrans menulis waktu dalam WIB (UTC+7) tanpa zona.
var wib = time.FixedZone("WIB", 7*3600)

// CreateQRIS membuat transaksi QRIS lewat Core API. Masa berlaku tidak diminta dari
// kita: batas waktu murni dari respons Midtrans (expiry_time), jadi hitung mundur di
// layar sama dengan yang berlaku di sisi penyedia.
func (m *Midtrans) CreateQRIS(orderID string, amount int, name, email string) (*QRIS, error) {
	if !m.Enabled() {
		return nil, errors.New("qris belum dikonfigurasi")
	}
	body, _ := json.Marshal(map[string]any{
		"payment_type":        "qris",
		"transaction_details": map[string]any{"order_id": orderID, "gross_amount": amount},
		"qris":                map[string]any{"acquirer": "gopay"},
		"customer_details":    map[string]any{"first_name": name, "email": email},
	})
	req, err := http.NewRequest(http.MethodPost, m.apiBase()+"/v2/charge", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.notifyURL != "" {
		req.Header.Set("X-Override-Notification", m.notifyURL)
	}
	started := time.Now()
	b, code, err := m.do(req)
	if err != nil {
		return nil, err
	}
	var out struct {
		StatusCode    string `json:"status_code"`
		StatusMessage string `json:"status_message"`
		QRString      string `json:"qr_string"`
		ExpiryTime    string `json:"expiry_time"`
		Actions       []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("respons qris tidak dikenali: %s", strings.TrimSpace(string(b)))
	}
	// Core API kadang menjawab HTTP 200 dengan status_code 4xx di badan respons.
	if code < 200 || code >= 300 || (out.StatusCode != "" && out.StatusCode != "200" && out.StatusCode != "201") {
		return nil, fmt.Errorf("qris %d/%s: %s", code, out.StatusCode, out.StatusMessage)
	}
	if out.QRString == "" {
		return nil, fmt.Errorf("respons qris tanpa qr_string: %s", strings.TrimSpace(string(b)))
	}
	imageURL := ""
	for _, a := range out.Actions {
		if a.Name == "generate-qr-code" {
			imageURL = a.URL
		}
	}
	if m.sandbox && imageURL != "" {
		log.Printf("[payment] sandbox, URL QR untuk simulator (order %s): %s", orderID, imageURL)
	}
	exp, err := time.ParseInLocation("2006-01-02 15:04:05", out.ExpiryTime, wib)
	if err != nil || !exp.After(started) {
		return nil, fmt.Errorf("respons qris tanpa expiry_time yang valid (%q)", out.ExpiryTime)
	}
	return &QRIS{QRString: out.QRString, ImageURL: imageURL, ExpiresAt: exp}, nil
}

// Status menanyakan status transaksi langsung ke Midtrans. Berguna saat backend
// berjalan lokal dan webhook tidak bisa menjangkau komputer kita.
func (m *Midtrans) Status(orderID string) (*Notification, error) {
	if !m.Enabled() {
		return nil, errors.New("midtrans belum dikonfigurasi")
	}
	req, err := http.NewRequest(http.MethodGet, m.apiBase()+"/v2/"+url.PathEscape(orderID)+"/status", nil)
	if err != nil {
		return nil, err
	}
	b, code, err := m.do(req)
	if err != nil {
		return nil, err
	}
	var n Notification
	if err := json.Unmarshal(b, &n); err != nil {
		return nil, fmt.Errorf("respons midtrans tidak dikenali: %s", strings.TrimSpace(string(b)))
	}
	// 404 = transaksi belum pernah dibayar / dibuka di Snap; bukan kegagalan sistem
	if n.TransactionStatus == "" {
		if code == http.StatusNotFound || n.StatusCode == "404" {
			return &Notification{OrderID: orderID, TransactionStatus: "pending"}, nil
		}
		return nil, fmt.Errorf("midtrans status %d: %s", code, strings.TrimSpace(string(b)))
	}
	return &n, nil
}
