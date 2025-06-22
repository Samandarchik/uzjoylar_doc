package main

import (
	"bytes"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
)

// Constants
const (
	SECRET_KEY                = "restaurant_secret_key_2024"
	ACCESS_TOKEN_EXPIRE_HOURS = 24
	TELEGRAM_BOT_TOKEN        = "7609705273:AAFoIawJBTGTFxECwhSjc7vpbgMBcveT_ko"
	TELEGRAM_GROUP_ID         = "-1002783983140"
	UPLOAD_DIR                = "uploads"
	MAX_FILE_SIZE             = 10 << 20 // 10MB
)

// Database instance
var db *sql.DB

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WebSocket clients
var clients = make(map[*websocket.Conn]bool)
var broadcast = make(chan []byte)

// Enums
type OrderStatus string

const (
	OrderPending   OrderStatus = "pending"
	OrderConfirmed OrderStatus = "confirmed"
	OrderPreparing OrderStatus = "preparing"
	OrderReady     OrderStatus = "ready"
	OrderDelivered OrderStatus = "delivered"
	OrderCancelled OrderStatus = "cancelled"
)

type DeliveryType string

const (
	DeliveryHome       DeliveryType = "delivery"
	DeliveryPickup     DeliveryType = "own_withdrawal"
	DeliveryRestaurant DeliveryType = "atTheRestaurant"
)

type PaymentMethod string

const (
	PaymentCash  PaymentMethod = "cash"
	PaymentCard  PaymentMethod = "card"
	PaymentClick PaymentMethod = "click"
	PaymentPayme PaymentMethod = "payme"
)

type PaymentStatus string

const (
	PaymentPending  PaymentStatus = "pending"
	PaymentPaid     PaymentStatus = "paid"
	PaymentFailed   PaymentStatus = "failed"
	PaymentRefunded PaymentStatus = "refunded"
)

// Multi-language support for foods only
var FOOD_TRANSLATIONS = map[string]map[string]string{
	"uz": {
		"shashlik":       "Shashlik",
		"milliy_taomlar": "Milliy taomlar",
		"ichimliklar":    "Ichimliklar",
		"salatlar":       "Salatlar",
		"shirinliklar":   "Shirinliklar",
	},
	"ru": {
		"shashlik":       "Шашлык",
		"milliy_taomlar": "Национальные блюда",
		"ichimliklar":    "Напитки",
		"salatlar":       "Салаты",
		"shirinliklar":   "Десерты",
	},
	"en": {
		"shashlik":       "Barbecue",
		"milliy_taomlar": "National dishes",
		"ichimliklar":    "Drinks",
		"salatlar":       "Salads",
		"shirinliklar":   "Desserts",
	},
}

// Models
type User struct {
	ID        string    `json:"id" db:"id"`
	Number    string    `json:"number" db:"number"`
	Password  string    `json:"password,omitempty" db:"password"`
	Role      string    `json:"role" db:"role"`
	FullName  string    `json:"full_name" db:"full_name"`
	Email     *string   `json:"email,omitempty" db:"email"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	TgID      *int64    `json:"tg_id,omitempty" db:"tg_id"`
	Language  string    `json:"language" db:"language"`
}

type Food struct {
	ID              int64               `json:"id" db:"id"`
	Names           map[string]string   `json:"names,omitempty" db:"names"`
	Name            string              `json:"name" db:"name"`
	Descriptions    map[string]string   `json:"descriptions,omitempty" db:"descriptions"`
	Description     string              `json:"description" db:"description"`
	Category        string              `json:"category" db:"category"`
	CategoryName    string              `json:"category_name,omitempty"`
	Price           int                 `json:"price" db:"price"`
	IsThere         bool                `json:"isThere" db:"is_there"`
	ImageURL        string              `json:"imageUrl" db:"image_url"`
	Ingredients     map[string][]string `json:"ingredients" db:"ingredients"`
	Allergens       map[string][]string `json:"allergens" db:"allergens"`
	Rating          float64             `json:"rating" db:"rating"`
	ReviewCount     int                 `json:"review_count" db:"review_count"`
	PreparationTime int                 `json:"preparation_time" db:"preparation_time"`
	Stock           int                 `json:"stock" db:"stock"`
	IsPopular       bool                `json:"is_popular" db:"is_popular"`
	Discount        int                 `json:"discount" db:"discount"`
	OriginalPrice   int                 `json:"original_price"`
	Comment         string              `json:"comment" db:"comment"`
	CreatedAt       time.Time           `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at" db:"updated_at"`
}

type OrderFood struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Price       int    `json:"price"`
	Description string `json:"description"`
	ImageURL    string `json:"imageUrl"`
	Count       int    `json:"count"`
	TotalPrice  int    `json:"total_price"`
}

type PaymentInfo struct {
	Method        PaymentMethod `json:"method"`
	Status        PaymentStatus `json:"status"`
	Amount        int           `json:"amount"`
	TransactionID *string       `json:"transaction_id,omitempty"`
	PaymentTime   *time.Time    `json:"payment_time,omitempty"`
}

type DeliveryInfo struct {
	Type       string   `json:"type"`
	Address    *string  `json:"address,omitempty"`
	Latitude   *float64 `json:"latitude,omitempty"`
	Longitude  *float64 `json:"longitude,omitempty"`
	Phone      *string  `json:"phone,omitempty"`
	TableID    *string  `json:"table_id,omitempty"`
	TableName  *string  `json:"table_name,omitempty"`
	PickupCode *string  `json:"pickup_code,omitempty"`
}

type Order struct {
	OrderID             string                 `json:"order_id" db:"order_id"`
	UserNumber          string                 `json:"user_number" db:"user_number"`
	UserName            string                 `json:"user_name" db:"user_name"`
	Foods               []OrderFood            `json:"foods" db:"foods"`
	TotalPrice          int                    `json:"total_price" db:"total_price"`
	OrderTime           time.Time              `json:"order_time" db:"order_time"`
	DeliveryType        string                 `json:"delivery_type" db:"delivery_type"`
	DeliveryInfo        map[string]interface{} `json:"delivery_info" db:"delivery_info"`
	Status              OrderStatus            `json:"status" db:"status"`
	PaymentInfo         PaymentInfo            `json:"payment_info" db:"payment_info"`
	SpecialInstructions *string                `json:"special_instructions,omitempty" db:"special_instructions"`
	EstimatedTime       *int                   `json:"estimated_time,omitempty" db:"estimated_time"`
	DeliveredAt         *time.Time             `json:"delivered_at,omitempty" db:"delivered_at"`
	StatusHistory       []StatusUpdate         `json:"status_history,omitempty" db:"status_history"`
	CreatedAt           time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at" db:"updated_at"`
}

type StatusUpdate struct {
	Status    OrderStatus `json:"status"`
	Timestamp time.Time   `json:"timestamp"`
	Note      string      `json:"note,omitempty"`
}

type Review struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	UserName  string    `json:"user_name,omitempty"`
	FoodID    int64     `json:"food_id" db:"food_id"`
	Rating    int       `json:"rating" db:"rating"`
	Comment   string    `json:"comment" db:"comment"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type FileUpload struct {
	ID           string    `json:"id" db:"id"`
	OriginalName string    `json:"original_name" db:"original_name"`
	FileName     string    `json:"file_name" db:"file_name"`
	FilePath     string    `json:"file_path" db:"file_path"`
	FileSize     int64     `json:"file_size" db:"file_size"`
	MimeType     string    `json:"mime_type" db:"mime_type"`
	URL          string    `json:"url" db:"url"`
	UploadedBy   string    `json:"uploaded_by" db:"uploaded_by"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// Request/Response structures
type LoginRequest struct {
	Number   string `json:"number" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RegisterRequest struct {
	Number   string  `json:"number" binding:"required"`
	Password string  `json:"password" binding:"required"`
	FullName string  `json:"full_name" binding:"required"`
	Email    *string `json:"email,omitempty"`
	TgID     *int64  `json:"tg_id,omitempty"`
	Language string  `json:"language,omitempty"`
}

type LoginResponse struct {
	Token    string `json:"token"`
	Role     string `json:"role"`
	UserID   string `json:"user_id"`
	Language string `json:"language"`
}

type FoodCreate struct {
	CustomID        *int64   `json:"custom_id,omitempty"`
	NameUz          string   `json:"nameUz" binding:"required"`
	NameRu          string   `json:"nameRu" binding:"required"`
	NameEn          string   `json:"nameEn" binding:"required"`
	DescriptionUz   string   `json:"descriptionUz" binding:"required"`
	DescriptionRu   string   `json:"descriptionRu" binding:"required"`
	DescriptionEn   string   `json:"descriptionEn" binding:"required"`
	Category        string   `json:"category" binding:"required"`
	Price           int      `json:"price" binding:"required"`
	IsThere         bool     `json:"isThere"`
	ImageURL        string   `json:"imageUrl"`
	IngredientsUz   []string `json:"ingredientsUz,omitempty"`
	IngredientsRu   []string `json:"ingredientsRu,omitempty"`
	IngredientsEn   []string `json:"ingredientsEn,omitempty"`
	AllergensUz     []string `json:"allergensUz,omitempty"`
	AllergensRu     []string `json:"allergensRu,omitempty"`
	AllergensEn     []string `json:"allergensEn,omitempty"`
	PreparationTime int      `json:"preparation_time,omitempty"`
	Stock           int      `json:"stock,omitempty"`
	IsPopular       bool     `json:"is_popular,omitempty"`
	Discount        int      `json:"discount,omitempty"`
	Comment         string   `json:"comment,omitempty"`
	StarRating      float64  `json:"star_rating,omitempty"`
}

type CartItem struct {
	FoodID   int64 `json:"food_id" binding:"required"`
	Quantity int   `json:"quantity" binding:"required,min=1"`
}

type OrderRequest struct {
	Items               []CartItem             `json:"items" binding:"required"`
	DeliveryType        DeliveryType           `json:"delivery_type" binding:"required"`
	DeliveryInfo        map[string]interface{} `json:"delivery_info"`
	PaymentMethod       PaymentMethod          `json:"payment_method" binding:"required"`
	SpecialInstructions *string                `json:"special_instructions,omitempty"`
	CustomerInfo        *CustomerInfo          `json:"customer_info,omitempty"`
}

type CustomerInfo struct {
	Name  string `json:"name,omitempty"`
	Phone string `json:"phone,omitempty"`
	Email string `json:"email,omitempty"`
}

type ReviewCreate struct {
	FoodID  int64  `json:"food_id" binding:"required"`
	Rating  int    `json:"rating" binding:"required,min=1,max=5"`
	Comment string `json:"comment" binding:"required"`
}

type ReviewUpdate struct {
	Rating  *int    `json:"rating,omitempty"`
	Comment *string `json:"comment,omitempty"`
}

// Telegram structures
type TelegramMessage struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

// JWT Claims
type Claims struct {
	Number string `json:"sub"`
	Role   string `json:"role"`
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// Restaurant tables
var RestaurantTables = map[string]string{
	// Zal-1
	"Zal-1 Stol-1":  "93e05d01c3304b3b9dc963db187dbb51",
	"Zal-1 Stol-2":  "73d6827a734a43b6ad779b5979bb9c6a",
	"Zal-1 Stol-3":  "dc6e76e87f9e42a08a4e1198fc5f89a0",
	"Zal-1 Stol-4":  "70a53b0ac3264fce88d9a4b7d3a7fa5e",
	"Zal-1 Stol-5":  "3b8bfb57a10b4e4cb3b7a6d1434dd1bc",
	"Zal-1 Stol-6":  "4f0e0220e40b43b5a28747984474d6f7",
	"Zal-1 Stol-7":  "15fc7ed2ff3041aeaa52c5087e51f6b2",
	"Zal-1 Stol-8":  "41d0d60382b246469b7e01d70031c648",
	"Zal-1 Stol-9":  "539f421ed1974f55b86d09cfdace9ae3",
	"Zal-1 Stol-10": "1ad401f487024d1ab78e1db90eb3ac18",
	"Zal-1 Stol-11": "367f6587c09d4c1ebfe2b3e31c45b0ec",
	"Zal-1 Stol-12": "da2a9f108bff460aa1b3149b8fa9ed2a",
	"Zal-1 Stol-13": "91e91fa5a9e849aab850152b55613f98",
	"Zal-1 Stol-14": "d6d2ee01a57f4f4e93e6788eb1ccf4b2",
	"Zal-1 Stol-15": "b0f79bb99fef4492a26573f279845b9c",
	"Zal-1 Stol-16": "c2b7aeef8e814a9c8dfc4935cf8392f6",
	"Zal-1 Stol-17": "f4389cde50ac4c2ab4487a4a106d6d48",

	// Zal-2
	"Zal-2 Stol-1":  "c366a08ac9aa48d4a29f31de3561f69a",
	"Zal-2 Stol-2":  "d10a58dcb3cc4e3eb67a84f785a1a62d",
	"Zal-2 Stol-3":  "ecfc541124a54051b78e72930e1eac54",
	"Zal-2 Stol-4":  "e5baf1c7ed4d4a449fca1c7df1bb7006",
	"Zal-2 Stol-5":  "22bc7dbd17e145c6be40b1d01b29b16d",
	"Zal-2 Stol-6":  "ff6c4b82207f42a89b676ec5d0f1f7cc",
	"Zal-2 Stol-7":  "f00db03ddfa24d8b9f603a59cfb6f6cf",
	"Zal-2 Stol-8":  "f5c5bfa4a9974643b7a3aeb6d1114c7b",
	"Zal-2 Stol-9":  "62eb05a6882c401c953933132d43b7ff",
	"Zal-2 Stol-10": "bb842ff325a8498a99414958c400bc62",
	"Zal-2 Stol-11": "5ab7550a5ecf49b2b28faec156acbd44",
	"Zal-2 Stol-12": "9d640accb3d94fcbad09c191f03a7f8e",
	"Zal-2 Stol-13": "7a4044a32e2b4a35a9c91be98c3975a2",
	"Zal-2 Stol-14": "9c45db6ccda54e989f8b0ebf12c0a34b",
	"Zal-2 Stol-15": "f3fbbf2f179b4ec89745bfc3fdd10667",
	"Zal-2 Stol-16": "42134cd30da04d5b9e37fc68f7913fc7",

	// Terassa
	"Terassa Stol-1":  "3066c1f1c2e640e5a7272e28b4d08f8e",
	"Terassa Stol-2":  "5932a6769b154a94b7dbbf646e3725a3",
	"Terassa Stol-3":  "bc1dce5a12d049a489f5aa6f7aa64b3c",
	"Terassa Stol-4":  "a30c8e82ab6843d898c487ae9a6f31f2",
	"Terassa Stol-5":  "fa8e703e17924a99b4496c96459ae1e7",
	"Terassa Stol-6":  "32575a40ab784b878888b1de5421c24f",
	"Terassa Stol-7":  "f4530dcf98854f92a49d64b71b7d1372",
	"Terassa Stol-8":  "93c931e153694f69a9fd404be85727de",
	"Terassa Stol-9":  "4be17f7c57964e689d536cc946925e02",
	"Terassa Stol-10": "1ad9d8bbcc4e4b58b90ffed835f42e6b",
	"Terassa Stol-11": "49045b8e013d4722a72a41e3a5b8a761",
	"Terassa Stol-12": "f9a753a6bfc5483f9be02b36b3a021ae",
	"Terassa Stol-13": "c4a91adbf5c545f0b5c2cd0732e429ef",
	"Terassa Stol-14": "be6e16140c744418b47e021134a31b3f",
	"Terassa Stol-15": "c3c2317de56f4f8da8fa4c758dfb0427",
	"Terassa Stol-16": "76a5f6e3c08d4761b859ea0bb496fc63",

	// VIP stollar
	"VIP-1": "vip1_id_placeholder",
	"VIP-2": "vip2_id_placeholder",
	"VIP-3": "vip3_id_placeholder",
	"VIP-4": "vip4_id_placeholder",
	"VIP-5": "vip5_id_placeholder",
	"VIP-6": "vip6_id_placeholder",
	"VIP-7": "vip7_id_placeholder",
}

// WebSocket message types
type WSMessage struct {
	Type    string      `json:"type"`
	Data    interface{} `json:"data"`
	OrderID string      `json:"order_id,omitempty"`
}

// ========== DATABASE FUNCTIONS ==========

func initDatabase() error {
	var err error

	// Database connection string
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5432"
	}
	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		dbUser = "postgres"
	}
	dbPassword := os.Getenv("DB_PASSWORD")
	if dbPassword == "" {
		dbPassword = "password"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "restaurant_db"
	}

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("database connection error: %v", err)
	}

	if err = db.Ping(); err != nil {
		return fmt.Errorf("database ping error: %v", err)
	}

	if err = createTables(); err != nil {
		return fmt.Errorf("create tables error: %v", err)
	}

	log.Println("✅ PostgreSQL database connected successfully")
	return nil
}

func createTables() error {
	queries := []string{
		// Users table
		`CREATE TABLE IF NOT EXISTS users (
			id VARCHAR(255) PRIMARY KEY,
			number VARCHAR(20) UNIQUE NOT NULL,
			password VARCHAR(255) NOT NULL,
			role VARCHAR(20) DEFAULT 'user',
			full_name VARCHAR(255) NOT NULL,
			email VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			is_active BOOLEAN DEFAULT true,
			tg_id BIGINT,
			language VARCHAR(5) DEFAULT 'uz'
		)`,

		// Foods table with BIGSERIAL ID
		`CREATE TABLE IF NOT EXISTS foods (
			id BIGSERIAL PRIMARY KEY,
			names JSONB,
			name VARCHAR(255) NOT NULL,
			descriptions JSONB,
			description TEXT,
			category VARCHAR(100) NOT NULL,
			price INTEGER NOT NULL,
			is_there BOOLEAN DEFAULT true,
			image_url TEXT,
			ingredients JSONB,
			allergens JSONB,
			rating DECIMAL(3,2) DEFAULT 0.0,
			review_count INTEGER DEFAULT 0,
			preparation_time INTEGER DEFAULT 15,
			stock INTEGER DEFAULT 100,
			is_popular BOOLEAN DEFAULT false,
			discount INTEGER DEFAULT 0,
			comment TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// Orders table
		`CREATE TABLE IF NOT EXISTS orders (
			order_id VARCHAR(255) PRIMARY KEY,
			user_number VARCHAR(20) NOT NULL,
			user_name VARCHAR(255) NOT NULL,
			foods JSONB NOT NULL,
			total_price INTEGER NOT NULL,
			order_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			delivery_type VARCHAR(50) NOT NULL,
			delivery_info JSONB,
			status VARCHAR(20) DEFAULT 'pending',
			payment_info JSONB NOT NULL,
			special_instructions TEXT,
			estimated_time INTEGER,
			delivered_at TIMESTAMP,
			status_history JSONB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// Reviews table
		`CREATE TABLE IF NOT EXISTS reviews (
			id VARCHAR(255) PRIMARY KEY,
			user_id VARCHAR(255) NOT NULL,
			food_id BIGINT NOT NULL,
			rating INTEGER NOT NULL CHECK (rating >= 1 AND rating <= 5),
			comment TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (food_id) REFERENCES foods(id) ON DELETE CASCADE,
			UNIQUE(user_id, food_id)
		)`,

		// File uploads table
		`CREATE TABLE IF NOT EXISTS file_uploads (
			id VARCHAR(255) PRIMARY KEY,
			original_name VARCHAR(255) NOT NULL,
			file_name VARCHAR(255) NOT NULL,
			file_path VARCHAR(500) NOT NULL,
			file_size BIGINT NOT NULL,
			mime_type VARCHAR(100) NOT NULL,
			url VARCHAR(500) NOT NULL,
			uploaded_by VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// Auto-update trigger for foods
		`CREATE OR REPLACE FUNCTION update_updated_at_column()
		RETURNS TRIGGER AS $$
		BEGIN
			NEW.updated_at = CURRENT_TIMESTAMP;
			RETURN NEW;
		END;
		$$ language 'plpgsql'`,

		`DROP TRIGGER IF EXISTS update_foods_updated_at ON foods`,

		`CREATE TRIGGER update_foods_updated_at 
			BEFORE UPDATE ON foods 
			FOR EACH ROW 
			EXECUTE FUNCTION update_updated_at_column()`,

		// Indexes
		`CREATE INDEX IF NOT EXISTS idx_foods_category ON foods(category)`,
		`CREATE INDEX IF NOT EXISTS idx_foods_is_there ON foods(is_there)`,
		`CREATE INDEX IF NOT EXISTS idx_foods_is_popular ON foods(is_popular)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_user_number ON orders(user_number)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_order_time ON orders(order_time)`,
		`CREATE INDEX IF NOT EXISTS idx_reviews_food_id ON reviews(food_id)`,
		`CREATE INDEX IF NOT EXISTS idx_reviews_user_id ON reviews(user_id)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("table creation error: %v", err)
		}
	}

	return nil
}

// ========== UTILITY FUNCTIONS ==========

func getFoodTranslation(key, lang string) string {
	if lang == "" {
		lang = "uz"
	}
	if translations, exists := FOOD_TRANSLATIONS[lang]; exists {
		if translation, exists := translations[key]; exists {
			return translation
		}
	}
	// Default uzbek language
	if translations, exists := FOOD_TRANSLATIONS["uz"]; exists {
		if translation, exists := translations[key]; exists {
			return translation
		}
	}
	return key
}

func getUserLanguage(headers map[string][]string) string {
	acceptLang := headers["Accept-Language"]
	if len(acceptLang) > 0 {
		lang := strings.Split(acceptLang[0], ",")[0]
		if strings.Contains(lang, "-") {
			lang = strings.Split(lang, "-")[0]
		}
		lang = strings.ToLower(lang)
		supportedLangs := []string{"uz", "ru", "en"}
		for _, supported := range supportedLangs {
			if lang == supported {
				return lang
			}
		}
	}
	return "uz"
}

func generateID(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, uuid.New().String()[:8])
}

func generateOrderID() string {
	today := time.Now().Format("2006-01-02")
	var count int
	query := `SELECT COUNT(*) FROM orders WHERE DATE(order_time) = $1`
	err := db.QueryRow(query, today).Scan(&count)
	if err != nil {
		log.Printf("Order count error: %v", err)
		count = 0
	}
	count++
	return fmt.Sprintf("%s-%d", today, count)
}

func hashPassword(password string) string {
	hash := md5.Sum([]byte(password))
	return fmt.Sprintf("%x", hash)
}

func createToken(user *User) (string, error) {
	expirationTime := time.Now().Add(ACCESS_TOKEN_EXPIRE_HOURS * time.Hour)
	claims := &Claims{
		Number: user.Number,
		Role:   user.Role,
		UserID: user.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(SECRET_KEY))
}

func getTableNameByID(tableID string) string {
	for tableName, id := range RestaurantTables {
		if id == tableID {
			return tableName
		}
	}
	return "Unknown table"
}

func getHostURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, c.Request.Host)
}

func cleanFileName(name string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9\-_.]`)
	cleaned := re.ReplaceAllString(name, "_")

	if len(cleaned) > 50 {
		ext := filepath.Ext(cleaned)
		nameWithoutExt := strings.TrimSuffix(cleaned, ext)
		if len(nameWithoutExt) > 46 {
			nameWithoutExt = nameWithoutExt[:46]
		}
		cleaned = nameWithoutExt + ext
	}

	return cleaned
}

// ========== FILE UPLOAD FUNCTIONS ==========

func uploadFile(c *gin.Context) {
	var uploaderID string
	if userInterface, exists := c.Get("user"); exists {
		user := userInterface.(*Claims)
		uploaderID = user.UserID
	}

	file, fileHeader, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file selected"})
		return
	}
	defer file.Close()

	if fileHeader.Size > MAX_FILE_SIZE {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large"})
		return
	}

	allowedTypes := map[string]bool{
		"image/jpeg": true,
		"image/jpg":  true,
		"image/png":  true,
		"image/gif":  true,
		"image/webp": true,
	}

	contentType := fileHeader.Header.Get("Content-Type")
	if !allowedTypes[contentType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file type"})
		return
	}

	if _, err := os.Stat(UPLOAD_DIR); os.IsNotExist(err) {
		os.MkdirAll(UPLOAD_DIR, 0755)
	}

	originalName := strings.TrimSuffix(fileHeader.Filename, filepath.Ext(fileHeader.Filename))
	cleanedName := cleanFileName(originalName)
	ext := filepath.Ext(fileHeader.Filename)

	fileName := fmt.Sprintf("%s_%d%s", cleanedName, time.Now().Unix(), ext)
	filePath := filepath.Join(UPLOAD_DIR, fileName)

	dst, err := os.Create(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "File save error"})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "File copy error"})
		return
	}

	fileURL := fmt.Sprintf("/uploads/%s", fileName)

	fileUpload := &FileUpload{
		ID:           generateID("file"),
		OriginalName: fileHeader.Filename,
		FileName:     fileName,
		FilePath:     filePath,
		FileSize:     fileHeader.Size,
		MimeType:     contentType,
		URL:          fileURL,
		UploadedBy:   uploaderID,
		CreatedAt:    time.Now(),
	}

	query := `INSERT INTO file_uploads (id, original_name, file_name, file_path, file_size, mime_type, url, uploaded_by, created_at) 
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = db.Exec(query, fileUpload.ID, fileUpload.OriginalName, fileUpload.FileName,
		fileUpload.FilePath, fileUpload.FileSize, fileUpload.MimeType, fileUpload.URL,
		fileUpload.UploadedBy, fileUpload.CreatedAt)

	if err != nil {
		log.Printf("File data save error: %v", err)
		os.Remove(filePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Data save error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "File uploaded successfully",
		"file":       fileUpload,
		"url":        fileURL,
		"public_url": fileURL,
	})
}

// ========== TELEGRAM BOT FUNCTIONS ==========

func sendTelegramMessage(message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", TELEGRAM_BOT_TOKEN)

	payload := TelegramMessage{
		ChatID: TELEGRAM_GROUP_ID,
		Text:   message,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API error: %s", string(body))
	}

	return nil
}

func sendTelegramMessageToUser(userTgID int64, message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", TELEGRAM_BOT_TOKEN)

	payload := TelegramMessage{
		ChatID: fmt.Sprintf("%d", userTgID),
		Text:   message,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API error: %s", string(body))
	}

	return nil
}

func formatOrderForTelegram(order *Order) string {
	message := fmt.Sprintf("🍽️ New Order!\n\n")
	message += fmt.Sprintf("📋 Order ID: %s\n", order.OrderID)
	message += fmt.Sprintf("👤 Customer: %s\n", order.UserName)
	message += fmt.Sprintf("📞 Phone: %s\n", order.UserNumber)
	message += fmt.Sprintf("🕐 Time: %s\n\n", order.OrderTime.Format("15:04"))

	message += fmt.Sprintf("🍕 Order Items:\n")
	for _, food := range order.Foods {
		message += fmt.Sprintf("• %s x%d = %d UZS\n", food.Name, food.Count, food.TotalPrice)
	}

	message += fmt.Sprintf("\n💰 Total Amount: %d UZS\n", order.TotalPrice)

	// Delivery information
	switch order.DeliveryType {
	case "delivery":
		if address, ok := order.DeliveryInfo["address"].(string); ok {
			message += fmt.Sprintf("🚚 Delivery Address: %s\n", address)
		}
		if lat, ok := order.DeliveryInfo["latitude"].(float64); ok {
			if lng, ok := order.DeliveryInfo["longitude"].(float64); ok {
				message += fmt.Sprintf("📍 Coordinates: %.6f, %.6f\n", lat, lng)
			}
		}
	case "own_withdrawal":
		message += fmt.Sprintf("🏪 Pickup\n")
	case "atTheRestaurant":
		if tableName, ok := order.DeliveryInfo["table_name"].(string); ok {
			message += fmt.Sprintf("🍽️ Table: %s\n", tableName)
		}
	}

	message += fmt.Sprintf("💳 Payment Method: %s\n", string(order.PaymentInfo.Method))

	if order.EstimatedTime != nil {
		message += fmt.Sprintf("⏱️ Preparation Time: %d minutes\n", *order.EstimatedTime)
	}

	if order.SpecialInstructions != nil && *order.SpecialInstructions != "" {
		message += fmt.Sprintf("📝 Special Instructions: %s\n", *order.SpecialInstructions)
	}

	return message
}

// ========== WEBSOCKET FUNCTIONS ==========

func handleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	clients[conn] = true
	log.Printf("WebSocket client connected. Total clients: %d", len(clients))

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			delete(clients, conn)
			log.Printf("WebSocket client disconnected. Remaining clients: %d", len(clients))
			break
		}
	}
}

func broadcastToClients(message WSMessage) {
	jsonData, _ := json.Marshal(message)
	for client := range clients {
		err := client.WriteJSON(message)
		if err != nil {
			log.Printf("WebSocket write error: %v", err)
			client.Close()
			delete(clients, client)
		}
	}
	log.Printf("WebSocket message sent: %s", string(jsonData))
}

func sendOrderUpdate(orderID string, status OrderStatus, message string) {
	wsMessage := WSMessage{
		Type:    "order_update",
		OrderID: orderID,
		Data: gin.H{
			"order_id": orderID,
			"status":   status,
			"message":  message,
			"time":     time.Now(),
		},
	}
	broadcastToClients(wsMessage)
}

func sendNewOrderNotification(order *Order) {
	wsMessage := WSMessage{
		Type:    "new_order",
		OrderID: order.OrderID,
		Data:    order,
	}
	broadcastToClients(wsMessage)

	// Send Telegram message to admin group
	go func() {
		telegramMessage := formatOrderForTelegram(order)
		if err := sendTelegramMessage(telegramMessage); err != nil {
			log.Printf("Telegram message error: %v", err)
		} else {
			log.Printf("Telegram message sent: %s", order.OrderID)
		}
	}()
}

// ========== MIDDLEWARE ==========

func corsMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})
}

func optionalAuthMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenString != authHeader {
				claims := &Claims{}
				token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
					return []byte(SECRET_KEY), nil
				})

				if err == nil && token.Valid {
					c.Set("user", claims)
				}
			}
		}
		c.Next()
	})
}

func authMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Bearer token required"})
			c.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(SECRET_KEY), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		c.Set("user", claims)
		c.Next()
	})
}

func adminMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
			c.Abort()
			return
		}

		claims := user.(*Claims)
		if claims.Role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
			c.Abort()
			return
		}

		c.Next()
	})
}

// ========== DATABASE HELPER FUNCTIONS ==========

func getUserByNumber(number string) (*User, error) {
	query := `SELECT id, number, password, role, full_name, email, created_at, is_active, tg_id, language 
			  FROM users WHERE number = $1`

	var user User
	err := db.QueryRow(query, number).Scan(
		&user.ID, &user.Number, &user.Password, &user.Role,
		&user.FullName, &user.Email, &user.CreatedAt,
		&user.IsActive, &user.TgID, &user.Language,
	)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

func createUser(user *User) error {
	query := `INSERT INTO users (id, number, password, role, full_name, email, created_at, is_active, tg_id, language) 
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := db.Exec(query, user.ID, user.Number, user.Password, user.Role,
		user.FullName, user.Email, user.CreatedAt, user.IsActive, user.TgID, user.Language)

	return err
}

func getFoodByID(foodID int64) (*Food, error) {
	query := `SELECT id, names, name, descriptions, description, category, price, is_there, 
			  image_url, ingredients, allergens, rating, review_count, preparation_time, 
			  stock, is_popular, discount, comment, created_at, updated_at
			  FROM foods WHERE id = $1`

	var food Food
	var namesJSON, descriptionsJSON, ingredientsJSON, allergensJSON []byte

	err := db.QueryRow(query, foodID).Scan(
		&food.ID, &namesJSON, &food.Name, &descriptionsJSON, &food.Description,
		&food.Category, &food.Price, &food.IsThere, &food.ImageURL,
		&ingredientsJSON, &allergensJSON, &food.Rating, &food.ReviewCount,
		&food.PreparationTime, &food.Stock, &food.IsPopular, &food.Discount,
		&food.Comment, &food.CreatedAt, &food.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	// JSON unmarshal
	if namesJSON != nil {
		json.Unmarshal(namesJSON, &food.Names)
	}
	if descriptionsJSON != nil {
		json.Unmarshal(descriptionsJSON, &food.Descriptions)
	}
	if ingredientsJSON != nil {
		json.Unmarshal(ingredientsJSON, &food.Ingredients)
	}
	if allergensJSON != nil {
		json.Unmarshal(allergensJSON, &food.Allergens)
	}

	return &food, nil
}

func getAllFoods() ([]*Food, error) {
	// Only available foods (isThere = true and stock > 0) ordered by ID
	query := `SELECT id, names, name, descriptions, description, category, price, is_there, 
			  image_url, ingredients, allergens, rating, review_count, preparation_time, 
			  stock, is_popular, discount, comment, created_at, updated_at
			  FROM foods WHERE is_there = true AND stock > 0 ORDER BY id ASC`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var foods []*Food
	for rows.Next() {
		var food Food
		var namesJSON, descriptionsJSON, ingredientsJSON, allergensJSON []byte

		err := rows.Scan(
			&food.ID, &namesJSON, &food.Name, &descriptionsJSON, &food.Description,
			&food.Category, &food.Price, &food.IsThere, &food.ImageURL,
			&ingredientsJSON, &allergensJSON, &food.Rating, &food.ReviewCount,
			&food.PreparationTime, &food.Stock, &food.IsPopular, &food.Discount,
			&food.Comment, &food.CreatedAt, &food.UpdatedAt,
		)

		if err != nil {
			continue
		}

		// JSON unmarshal
		if namesJSON != nil {
			json.Unmarshal(namesJSON, &food.Names)
		}
		if descriptionsJSON != nil {
			json.Unmarshal(descriptionsJSON, &food.Descriptions)
		}
		if ingredientsJSON != nil {
			json.Unmarshal(ingredientsJSON, &food.Ingredients)
		}
		if allergensJSON != nil {
			json.Unmarshal(allergensJSON, &food.Allergens)
		}

		foods = append(foods, &food)
	}

	return foods, nil
}

func getAllFoodsForAdmin() ([]*Food, error) {
	// All foods for admin ordered by ID
	query := `SELECT id, names, name, descriptions, description, category, price, is_there, 
			  image_url, ingredients, allergens, rating, review_count, preparation_time, 
			  stock, is_popular, discount, comment, created_at, updated_at
			  FROM foods ORDER BY id ASC`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var foods []*Food
	for rows.Next() {
		var food Food
		var namesJSON, descriptionsJSON, ingredientsJSON, allergensJSON []byte

		err := rows.Scan(
			&food.ID, &namesJSON, &food.Name, &descriptionsJSON, &food.Description,
			&food.Category, &food.Price, &food.IsThere, &food.ImageURL,
			&ingredientsJSON, &allergensJSON, &food.Rating, &food.ReviewCount,
			&food.PreparationTime, &food.Stock, &food.IsPopular, &food.Discount,
			&food.Comment, &food.CreatedAt, &food.UpdatedAt,
		)

		if err != nil {
			continue
		}

		// JSON unmarshal
		if namesJSON != nil {
			json.Unmarshal(namesJSON, &food.Names)
		}
		if descriptionsJSON != nil {
			json.Unmarshal(descriptionsJSON, &food.Descriptions)
		}
		if ingredientsJSON != nil {
			json.Unmarshal(ingredientsJSON, &food.Ingredients)
		}
		if allergensJSON != nil {
			json.Unmarshal(allergensJSON, &food.Allergens)
		}

		foods = append(foods, &food)
	}

	return foods, nil
}

func createFoodWithCustomID(food *Food, customID *int64) error {
	var query string
	var err error

	namesJSON, err := json.Marshal(food.Names)
	if err != nil {
		return fmt.Errorf("names JSON marshal error: %w", err)
	}

	descriptionsJSON, err := json.Marshal(food.Descriptions)
	if err != nil {
		return fmt.Errorf("descriptions JSON marshal error: %w", err)
	}

	ingredientsJSON, err := json.Marshal(food.Ingredients)
	if err != nil {
		return fmt.Errorf("ingredients JSON marshal error: %w", err)
	}

	allergensJSON, err := json.Marshal(food.Allergens)
	if err != nil {
		return fmt.Errorf("allergens JSON marshal error: %w", err)
	}

	if customID != nil {
		// Manual ID insertion
		query = `INSERT INTO foods (id, names, name, descriptions, description, category, price, 
				 is_there, image_url, ingredients, allergens, rating, review_count, 
				 preparation_time, stock, is_popular, discount, comment, created_at, updated_at) 
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`

		_, err = db.Exec(query, *customID, namesJSON, food.Name, descriptionsJSON, food.Description,
			food.Category, food.Price, food.IsThere, food.ImageURL, ingredientsJSON,
			allergensJSON, food.Rating, food.ReviewCount, food.PreparationTime,
			food.Stock, food.IsPopular, food.Discount, food.Comment,
			food.CreatedAt, food.UpdatedAt)

		if err != nil {
			return fmt.Errorf("manual ID insertion error: %w", err)
		}

		food.ID = *customID
	} else {
		// Automatic ID (BIGSERIAL)
		query = `INSERT INTO foods (names, name, descriptions, description, category, price, 
				 is_there, image_url, ingredients, allergens, rating, review_count, 
				 preparation_time, stock, is_popular, discount, comment, created_at, updated_at) 
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
				 RETURNING id`

		err = db.QueryRow(query, namesJSON, food.Name, descriptionsJSON, food.Description,
			food.Category, food.Price, food.IsThere, food.ImageURL, ingredientsJSON,
			allergensJSON, food.Rating, food.ReviewCount, food.PreparationTime,
			food.Stock, food.IsPopular, food.Discount, food.Comment,
			food.CreatedAt, food.UpdatedAt).Scan(&food.ID)

		if err != nil {
			return fmt.Errorf("automatic ID insertion error: %w", err)
		}
	}

	log.Printf("✅ Food created successfully with ID: %d", food.ID)
	return nil
}

func updateFood(food *Food) error {
	namesJSON, _ := json.Marshal(food.Names)
	descriptionsJSON, _ := json.Marshal(food.Descriptions)
	ingredientsJSON, _ := json.Marshal(food.Ingredients)
	allergensJSON, _ := json.Marshal(food.Allergens)

	query := `UPDATE foods SET names = $2, name = $3, descriptions = $4, description = $5, 
			  category = $6, price = $7, is_there = $8, image_url = $9, ingredients = $10, 
			  allergens = $11, rating = $12, review_count = $13, preparation_time = $14, 
			  stock = $15, is_popular = $16, discount = $17, comment = $18, updated_at = CURRENT_TIMESTAMP
			  WHERE id = $1`

	_, err := db.Exec(query, food.ID, namesJSON, food.Name, descriptionsJSON, food.Description,
		food.Category, food.Price, food.IsThere, food.ImageURL, ingredientsJSON,
		allergensJSON, food.Rating, food.ReviewCount, food.PreparationTime,
		food.Stock, food.IsPopular, food.Discount, food.Comment)

	return err
}

func createOrder(order *Order) error {
	foodsJSON, _ := json.Marshal(order.Foods)
	deliveryInfoJSON, _ := json.Marshal(order.DeliveryInfo)
	paymentInfoJSON, _ := json.Marshal(order.PaymentInfo)
	statusHistoryJSON, _ := json.Marshal(order.StatusHistory)

	query := `INSERT INTO orders (order_id, user_number, user_name, foods, total_price, 
			  order_time, delivery_type, delivery_info, status, payment_info, 
			  special_instructions, estimated_time, delivered_at, status_history, 
			  created_at, updated_at) 
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`

	_, err := db.Exec(query, order.OrderID, order.UserNumber, order.UserName, foodsJSON,
		order.TotalPrice, order.OrderTime, order.DeliveryType, deliveryInfoJSON,
		order.Status, paymentInfoJSON, order.SpecialInstructions, order.EstimatedTime,
		order.DeliveredAt, statusHistoryJSON, order.CreatedAt, order.UpdatedAt)

	return err
}

func getOrderByID(orderID string) (*Order, error) {
	query := `SELECT order_id, user_number, user_name, foods, total_price, order_time, 
			  delivery_type, delivery_info, status, payment_info, special_instructions, 
			  estimated_time, delivered_at, status_history, created_at, updated_at
			  FROM orders WHERE order_id = $1`

	var order Order
	var foodsJSON, deliveryInfoJSON, paymentInfoJSON, statusHistoryJSON []byte

	err := db.QueryRow(query, orderID).Scan(
		&order.OrderID, &order.UserNumber, &order.UserName, &foodsJSON,
		&order.TotalPrice, &order.OrderTime, &order.DeliveryType, &deliveryInfoJSON,
		&order.Status, &paymentInfoJSON, &order.SpecialInstructions,
		&order.EstimatedTime, &order.DeliveredAt, &statusHistoryJSON,
		&order.CreatedAt, &order.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	// JSON unmarshal
	json.Unmarshal(foodsJSON, &order.Foods)
	json.Unmarshal(deliveryInfoJSON, &order.DeliveryInfo)
	json.Unmarshal(paymentInfoJSON, &order.PaymentInfo)
	json.Unmarshal(statusHistoryJSON, &order.StatusHistory)

	return &order, nil
}

func updateOrder(order *Order) error {
	foodsJSON, _ := json.Marshal(order.Foods)
	deliveryInfoJSON, _ := json.Marshal(order.DeliveryInfo)
	paymentInfoJSON, _ := json.Marshal(order.PaymentInfo)
	statusHistoryJSON, _ := json.Marshal(order.StatusHistory)

	query := `UPDATE orders SET user_number = $2, user_name = $3, foods = $4, total_price = $5, 
			  order_time = $6, delivery_type = $7, delivery_info = $8, status = $9, 
			  payment_info = $10, special_instructions = $11, estimated_time = $12, 
			  delivered_at = $13, status_history = $14, updated_at = CURRENT_TIMESTAMP
			  WHERE order_id = $1`

	_, err := db.Exec(query, order.OrderID, order.UserNumber, order.UserName, foodsJSON,
		order.TotalPrice, order.OrderTime, order.DeliveryType, deliveryInfoJSON,
		order.Status, paymentInfoJSON, order.SpecialInstructions, order.EstimatedTime,
		order.DeliveredAt, statusHistoryJSON)

	return err
}

func updateSequenceAfterManualInsert() error {
	query := `SELECT setval('foods_id_seq', (SELECT MAX(id) FROM foods))`
	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("sequence update error: %w", err)
	}
	log.Println("✅ Sequence updated")
	return nil
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}

func getLocalizedFood(food *Food, lang string) *Food {
	localizedFood := *food

	// Get multilingual name
	if food.Names != nil {
		if name, exists := food.Names[lang]; exists {
			localizedFood.Name = name
		} else if name, exists := food.Names["uz"]; exists {
			localizedFood.Name = name
		}
	}

	// Get multilingual description
	if food.Descriptions != nil {
		if desc, exists := food.Descriptions[lang]; exists {
			localizedFood.Description = desc
		} else if desc, exists := food.Descriptions["uz"]; exists {
			localizedFood.Description = desc
		}
	}

	// Translate category name
	categoryKey := strings.ToLower(strings.ReplaceAll(food.Category, " ", "_"))
	localizedFood.CategoryName = getFoodTranslation(categoryKey, lang)

	// Calculate discount
	if food.Discount > 0 {
		localizedFood.OriginalPrice = food.Price
		localizedFood.Price = food.Price - (food.Price * food.Discount / 100)
	}

	return &localizedFood
}

func getAllLocalizedFoods(lang string, isAdmin bool) ([]*Food, error) {
	var foods []*Food
	var err error

	if isAdmin {
		foods, err = getAllFoodsForAdmin()
	} else {
		foods, err = getAllFoods()
	}

	if err != nil {
		return nil, err
	}

	var localizedFoods []*Food
	for _, food := range foods {
		localizedFood := getLocalizedFood(food, lang)
		localizedFoods = append(localizedFoods, localizedFood)
	}

	return localizedFoods, nil
}

// ========== AUTHENTICATION HANDLERS ==========

func register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	lang := req.Language
	if lang == "" {
		lang = "uz"
	}

	// Check if user exists
	_, err := getUserByNumber(req.Number)
	if err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Phone number already registered"})
		return
	}

	user := &User{
		ID:        generateID("user"),
		Number:    req.Number,
		Password:  hashPassword(req.Password),
		Role:      "user",
		FullName:  req.FullName,
		Email:     req.Email,
		CreatedAt: time.Now(),
		IsActive:  true,
		TgID:      req.TgID,
		Language:  lang,
	}

	if err := createUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User creation error"})
		return
	}

	token, err := createToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token creation error"})
		return
	}

	response := LoginResponse{
		Token:    token,
		Role:     user.Role,
		UserID:   user.ID,
		Language: lang,
	}

	c.JSON(http.StatusOK, response)
}

func login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := getUserByNumber(req.Number)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	if user.Password != hashPassword(req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	token, err := createToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token creation error"})
		return
	}

	response := LoginResponse{
		Token:    token,
		Role:     user.Role,
		UserID:   user.ID,
		Language: user.Language,
	}

	c.JSON(http.StatusOK, response)
}

func getProfile(c *gin.Context) {
	user := c.MustGet("user").(*Claims)

	userDB, err := getUserByNumber(user.Number)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Remove password
	userResponse := *userDB
	userResponse.Password = ""

	c.JSON(http.StatusOK, userResponse)
}

// ========== CATEGORY HANDLERS ==========

func getCategories(c *gin.Context) {
	lang := getUserLanguage(c.Request.Header)

	categories := []gin.H{
		{"key": "shashlik", "name": getFoodTranslation("shashlik", lang)},
		{"key": "milliy_taomlar", "name": getFoodTranslation("milliy_taomlar", lang)},
		{"key": "ichimliklar", "name": getFoodTranslation("ichimliklar", lang)},
		{"key": "salatlar", "name": getFoodTranslation("salatlar", lang)},
		{"key": "shirinliklar", "name": getFoodTranslation("shirinliklar", lang)},
	}

	c.JSON(http.StatusOK, categories)
}

// ========== FOOD HANDLERS ==========

func getAllFoodsHandler(c *gin.Context) {
	lang := getUserLanguage(c.Request.Header)
	category := c.Query("category")
	search := c.Query("search")
	popular := c.Query("popular")
	sortBy := c.Query("sort")
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	// Check if admin or regular user
	isAdmin := false
	if userInterface, exists := c.Get("user"); exists {
		user := userInterface.(*Claims)
		isAdmin = (user.Role == "admin")
	}

	foods, err := getAllLocalizedFoods(lang, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Data fetch error"})
		return
	}

	// Filtering
	if category != "" {
		filtered := []*Food{}
		for _, food := range foods {
			if strings.ToLower(food.Category) == strings.ToLower(category) {
				filtered = append(filtered, food)
			}
		}
		foods = filtered
	}

	if search != "" {
		searchLower := strings.ToLower(search)
		filtered := []*Food{}
		for _, food := range foods {
			if strings.Contains(strings.ToLower(food.Name), searchLower) ||
				strings.Contains(strings.ToLower(food.Description), searchLower) {
				filtered = append(filtered, food)
			}
		}
		foods = filtered
	}

	if popular == "true" {
		filtered := []*Food{}
		for _, food := range foods {
			if food.IsPopular {
				filtered = append(filtered, food)
			}
		}
		foods = filtered
	}

	// Sorting
	switch sortBy {
	case "price_asc":
		sort.Slice(foods, func(i, j int) bool {
			return foods[i].Price < foods[j].Price
		})
	case "price_desc":
		sort.Slice(foods, func(i, j int) bool {
			return foods[i].Price > foods[j].Price
		})
	case "rating":
		sort.Slice(foods, func(i, j int) bool {
			return foods[i].Rating > foods[j].Rating
		})
	case "popular":
		sort.Slice(foods, func(i, j int) bool {
			if foods[i].IsPopular != foods[j].IsPopular {
				return foods[i].IsPopular
			}
			return foods[i].Rating > foods[j].Rating
		})
	case "name":
		sort.Slice(foods, func(i, j int) bool {
			return foods[i].Name < foods[j].Name
		})
	default:
		// Default: ID ascending (1, 2, 3, 4...)
		sort.Slice(foods, func(i, j int) bool {
			return foods[i].ID < foods[j].ID
		})
	}

	// Pagination
	total := len(foods)
	start := (page - 1) * limit
	end := start + limit
	if start >= total {
		foods = []*Food{}
	} else {
		if end > total {
			end = total
		}
		foods = foods[start:end]
	}

	// Make ImageURL full URL
	hostURL := getHostURL(c)
	for _, food := range foods {
		if food.ImageURL != "" && !strings.HasPrefix(food.ImageURL, "http") {
			food.ImageURL = hostURL + food.ImageURL
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"foods": foods,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": (total + limit - 1) / limit,
		},
	})
}

func getFoodHandler(c *gin.Context) {
	foodIDStr := c.Param("food_id")
	lang := getUserLanguage(c.Request.Header)

	// Convert string ID to integer
	foodID, err := strconv.ParseInt(foodIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid food ID format"})
		return
	}

	food, err := getFoodByID(foodID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Food not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Data fetch error"})
		return
	}

	localizedFood := getLocalizedFood(food, lang)

	// Make ImageURL full URL
	if localizedFood.ImageURL != "" && !strings.HasPrefix(localizedFood.ImageURL, "http") {
		localizedFood.ImageURL = getHostURL(c) + localizedFood.ImageURL
	}

	c.JSON(http.StatusOK, localizedFood)
}

func createFoodHandler(c *gin.Context) {
	var req FoodCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("JSON binding error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid JSON format",
			"details": err.Error(),
		})
		return
	}

	log.Printf("Received food creation request: %+v", req)

	// Check if custom ID already exists
	if req.CustomID != nil {
		var exists bool
		checkQuery := `SELECT EXISTS(SELECT 1 FROM foods WHERE id = $1)`
		err := db.QueryRow(checkQuery, *req.CustomID).Scan(&exists)
		if err != nil {
			log.Printf("ID check error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "ID check error",
				"details": err.Error(),
			})
			return
		}
		if exists {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("ID %d already exists", *req.CustomID)})
			return
		}
	}

	// Set defaults
	if req.PreparationTime == 0 {
		req.PreparationTime = 15
	}
	if req.Stock == 0 {
		req.Stock = 100
	}

	// Create multilingual names
	names := map[string]string{
		"uz": req.NameUz,
		"ru": req.NameRu,
		"en": req.NameEn,
	}

	// Create multilingual descriptions
	descriptions := map[string]string{
		"uz": req.DescriptionUz,
		"ru": req.DescriptionRu,
		"en": req.DescriptionEn,
	}

	// Create multilingual ingredients
	ingredients := map[string][]string{
		"uz": req.IngredientsUz,
		"ru": req.IngredientsRu,
		"en": req.IngredientsEn,
	}

	// Create multilingual allergens
	allergens := map[string][]string{
		"uz": req.AllergensUz,
		"ru": req.AllergensRu,
		"en": req.AllergensEn,
	}

	food := &Food{
		Names:           names,
		Name:            req.NameUz, // Default Uzbek
		Descriptions:    descriptions,
		Description:     req.DescriptionUz, // Default Uzbek
		Category:        req.Category,
		Price:           req.Price,
		IsThere:         req.IsThere,
		ImageURL:        req.ImageURL,
		Ingredients:     ingredients,
		Allergens:       allergens,
		Rating:          req.StarRating, // Use star_rating from request
		ReviewCount:     0,
		PreparationTime: req.PreparationTime,
		Stock:           req.Stock,
		IsPopular:       req.IsPopular,
		Discount:        req.Discount,
		Comment:         req.Comment,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	log.Printf("Food object created: %+v", food)

	if err := createFoodWithCustomID(food, req.CustomID); err != nil {
		log.Printf("Food creation error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Food creation error",
			"details": err.Error(),
		})
		return
	}

	// Update sequence if custom ID was used
	if req.CustomID != nil {
		if err := updateSequenceAfterManualInsert(); err != nil {
			log.Printf("Sequence update warning: %v", err)
		}
	}

	log.Printf("Food created successfully: ID=%d, Name=%s", food.ID, food.Name)

	c.JSON(http.StatusCreated, gin.H{
		"message": "Food created successfully",
		"food":    food,
		"id_type": map[bool]string{true: "custom", false: "auto"}[req.CustomID != nil],
	})
}

func updateFoodHandler(c *gin.Context) {
	foodIDStr := c.Param("food_id")

	// Convert string ID to integer
	foodID, err := strconv.ParseInt(foodIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid food ID format"})
		return
	}

	food, err := getFoodByID(foodID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Food not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Data fetch error"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update fields
	if name, ok := updates["name"].(string); ok {
		food.Name = name
	}
	if category, ok := updates["category"].(string); ok {
		food.Category = category
	}
	if price, ok := updates["price"].(float64); ok {
		food.Price = int(price)
	}
	if description, ok := updates["description"].(string); ok {
		food.Description = description
	}
	if isThere, ok := updates["isThere"].(bool); ok {
		food.IsThere = isThere
	}
	if imageURL, ok := updates["imageUrl"].(string); ok {
		food.ImageURL = imageURL
	}
	if prepTime, ok := updates["preparation_time"].(float64); ok {
		food.PreparationTime = int(prepTime)
	}
	if stock, ok := updates["stock"].(float64); ok {
		food.Stock = int(stock)
	}
	if isPopular, ok := updates["is_popular"].(bool); ok {
		food.IsPopular = isPopular
	}
	if discount, ok := updates["discount"].(float64); ok {
		food.Discount = int(discount)
	}

	food.UpdatedAt = time.Now()

	if err := updateFood(food); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Food update error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Food updated successfully",
		"food":    food,
	})
}

func deleteFoodHandler(c *gin.Context) {
	foodIDStr := c.Param("food_id")

	// Convert string ID to integer
	foodID, err := strconv.ParseInt(foodIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid food ID format"})
		return
	}

	// Check if food exists
	_, err = getFoodByID(foodID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Food not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Data fetch error"})
		return
	}

	// Delete food
	query := `DELETE FROM foods WHERE id = $1`
	_, err = db.Exec(query, foodID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Food deletion error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Food deleted successfully"})
}

// ========== ORDER HANDLERS ==========

func createOrderHandler(c *gin.Context) {
	var req OrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// User authentication
	var user *Claims
	if userInterface, exists := c.Get("user"); exists {
		user = userInterface.(*Claims)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Login required"})
		return
	}

	// Check cart
	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cart is empty"})
		return
	}

	log.Printf("Creating order for user: %s, items count: %d", user.Number, len(req.Items))

	// Check foods and stock
	var orderedFoods []OrderFood
	totalPrice := 0
	totalPrepTime := 0

	for _, item := range req.Items {
		log.Printf("Processing food_id: %d, quantity: %d", item.FoodID, item.Quantity)

		food, err := getFoodByID(item.FoodID)
		if err != nil {
			log.Printf("Food not found error: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Food not available",
				"food_id": item.FoodID,
				"details": err.Error(),
			})
			return
		}

		if !food.IsThere || food.Stock <= 0 {
			log.Printf("Food not available: isThere=%v, stock=%d", food.IsThere, food.Stock)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Food not available",
				"food_id": item.FoodID,
			})
			return
		}

		// Check stock
		if food.Stock < item.Quantity {
			log.Printf("Insufficient stock: required=%d, available=%d", item.Quantity, food.Stock)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":     "Insufficient stock",
				"food_id":   item.FoodID,
				"required":  item.Quantity,
				"available": food.Stock,
			})
			return
		}

		localizedFood := getLocalizedFood(food, "uz")
		foodTotalPrice := localizedFood.Price * item.Quantity
		prepTime := food.PreparationTime
		if prepTime > totalPrepTime {
			totalPrepTime = prepTime
		}

		orderedFood := OrderFood{
			ID:          food.ID,
			Name:        localizedFood.Name,
			Category:    localizedFood.CategoryName,
			Price:       localizedFood.Price,
			Description: localizedFood.Description,
			ImageURL:    localizedFood.ImageURL,
			Count:       item.Quantity,
			TotalPrice:  foodTotalPrice,
		}
		orderedFoods = append(orderedFoods, orderedFood)
		totalPrice += foodTotalPrice

		// Reduce stock
		food.Stock -= item.Quantity
		if err := updateFood(food); err != nil {
			log.Printf("Stock update error: %v", err)
		}
	}

	log.Printf("Order foods processed successfully, total_price: %d", totalPrice)

	// Delivery information
	deliveryInfo := make(map[string]interface{})
	switch req.DeliveryType {
	case DeliveryHome:
		address, addressOk := req.DeliveryInfo["address"].(string)
		if !addressOk || address == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Delivery address required"})
			return
		}

		deliveryInfo = map[string]interface{}{
			"type":    "delivery",
			"address": address,
		}

		if phone, ok := req.DeliveryInfo["phone"].(string); ok {
			deliveryInfo["phone"] = phone
		}

		if lat, ok := req.DeliveryInfo["latitude"].(float64); ok {
			deliveryInfo["latitude"] = lat
		}
		if lng, ok := req.DeliveryInfo["longitude"].(float64); ok {
			deliveryInfo["longitude"] = lng
		}

		totalPrepTime += 20 // delivery time
	case DeliveryPickup:
		deliveryInfo = map[string]interface{}{
			"type":        "own_withdrawal",
			"pickup_code": generateID("pickup"),
		}
	case DeliveryRestaurant:
		tableID, ok := req.DeliveryInfo["table_id"].(string)
		if !ok || tableID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Table ID required"})
			return
		}
		tableName := getTableNameByID(tableID)
		deliveryInfo = map[string]interface{}{
			"type":       "atTheRestaurant",
			"table_id":   tableID,
			"table_name": tableName,
		}
	}

	log.Printf("Delivery info prepared: %+v", deliveryInfo)

	// Payment information
	paymentInfo := PaymentInfo{
		Method: req.PaymentMethod,
		Status: PaymentPending,
		Amount: totalPrice,
	}

	if req.PaymentMethod != PaymentCash {
		transactionID := generateID("txn")
		paymentInfo.TransactionID = &transactionID
	}

	log.Printf("Payment info prepared: %+v", paymentInfo)

	// Create order
	orderID := generateOrderID()
	orderTime := time.Now()

	userDB, _ := getUserByNumber(user.Number)
	userName := "User"
	if userDB != nil {
		userName = userDB.FullName
	}

	// Use customer info
	if req.CustomerInfo != nil {
		if req.CustomerInfo.Name != "" {
			userName = req.CustomerInfo.Name
		}
	}

	log.Printf("Order ID generated: %s", orderID)

	order := &Order{
		OrderID:             orderID,
		UserNumber:          user.Number,
		UserName:            userName,
		Foods:               orderedFoods,
		TotalPrice:          totalPrice,
		OrderTime:           orderTime,
		DeliveryType:        string(req.DeliveryType),
		DeliveryInfo:        deliveryInfo,
		Status:              OrderPending,
		PaymentInfo:         paymentInfo,
		SpecialInstructions: req.SpecialInstructions,
		EstimatedTime:       &totalPrepTime,
		StatusHistory: []StatusUpdate{
			{
				Status:    OrderPending,
				Timestamp: orderTime,
				Note:      "Order created",
			},
		},
		CreatedAt: orderTime,
		UpdatedAt: orderTime,
	}

	log.Printf("Order object created, attempting to save to database...")

	if err := createOrder(order); err != nil {
		log.Printf("Database error creating order: %v", err)

		// Rollback stock
		for _, item := range req.Items {
			if food, err := getFoodByID(item.FoodID); err == nil {
				food.Stock += item.Quantity
				updateFood(food)
			}
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Order creation error",
			"details": err.Error(),
		})
		return
	}

	log.Printf("Order created successfully in database: %s", orderID)

	// Real-time update
	go func() {
		sendNewOrderNotification(order)
		sendOrderUpdate(orderID, OrderPending, "Order created")
	}()

	c.JSON(http.StatusCreated, gin.H{
		"order":          order,
		"message":        "Order created successfully",
		"estimated_time": totalPrepTime,
		"order_tracking": fmt.Sprintf("/api/orders/%s/track", orderID),
	})
}

func getOrdersHandler(c *gin.Context) {
	user := c.MustGet("user").(*Claims)
	status := c.Query("status")
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}

	offset := (page - 1) * limit

	// Build query
	var query string
	var args []interface{}
	argIndex := 1

	if user.Role == "admin" {
		query = `SELECT order_id, user_number, user_name, foods, total_price, order_time, 
				 delivery_type, delivery_info, status, payment_info, special_instructions, 
				 estimated_time, delivered_at, status_history, created_at, updated_at
				 FROM orders WHERE 1=1`
		if status != "" {
			query += fmt.Sprintf(" AND status = $%d", argIndex)
			args = append(args, status)
			argIndex++
		}
	} else {
		query = `SELECT order_id, user_number, user_name, foods, total_price, order_time, 
				 delivery_type, delivery_info, status, payment_info, special_instructions, 
				 estimated_time, delivered_at, status_history, created_at, updated_at
				 FROM orders WHERE user_number = $1`
		args = append(args, user.Number)
		argIndex++
		if status != "" {
			query += fmt.Sprintf(" AND status = $%d", argIndex)
			args = append(args, status)
			argIndex++
		}
	}

	query += fmt.Sprintf(" ORDER BY order_time DESC LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Data fetch error"})
		return
	}
	defer rows.Close()

	var orders []*Order
	for rows.Next() {
		var order Order
		var foodsJSON, deliveryInfoJSON, paymentInfoJSON, statusHistoryJSON []byte

		err := rows.Scan(
			&order.OrderID, &order.UserNumber, &order.UserName, &foodsJSON,
			&order.TotalPrice, &order.OrderTime, &order.DeliveryType, &deliveryInfoJSON,
			&order.Status, &paymentInfoJSON, &order.SpecialInstructions,
			&order.EstimatedTime, &order.DeliveredAt, &statusHistoryJSON,
			&order.CreatedAt, &order.UpdatedAt,
		)

		if err != nil {
			continue
		}

		// JSON unmarshal
		json.Unmarshal(foodsJSON, &order.Foods)
		json.Unmarshal(deliveryInfoJSON, &order.DeliveryInfo)
		json.Unmarshal(paymentInfoJSON, &order.PaymentInfo)
		json.Unmarshal(statusHistoryJSON, &order.StatusHistory)

		orders = append(orders, &order)
	}

	// Total count
	var countQuery string
	var countArgs []interface{}
	if user.Role == "admin" {
		countQuery = `SELECT COUNT(*) FROM orders`
		if status != "" {
			countQuery += " WHERE status = $1"
			countArgs = append(countArgs, status)
		}
	} else {
		countQuery = `SELECT COUNT(*) FROM orders WHERE user_number = $1`
		countArgs = append(countArgs, user.Number)
		if status != "" {
			countQuery += " AND status = $2"
			countArgs = append(countArgs, status)
		}
	}

	var total int
	db.QueryRow(countQuery, countArgs...).Scan(&total)

	c.JSON(http.StatusOK, gin.H{
		"orders": orders,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": (total + limit - 1) / limit,
		},
	})
}

func getOrderHandler(c *gin.Context) {
	orderID := c.Param("order_id")
	user := c.MustGet("user").(*Claims)

	order, err := getOrderByID(orderID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Data fetch error"})
		return
	}

	// User can only see their own orders
	if user.Role != "admin" && order.UserNumber != user.Number {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, order)
}

func updateOrderStatusHandler(c *gin.Context) {
	orderID := c.Param("order_id")

	var req struct {
		Status OrderStatus `json:"status" binding:"required"`
		Note   string      `json:"note,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	order, err := getOrderByID(orderID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Data fetch error"})
		return
	}

	// Add to status history
	statusUpdate := StatusUpdate{
		Status:    req.Status,
		Timestamp: time.Now(),
		Note:      req.Note,
	}
	order.StatusHistory = append(order.StatusHistory, statusUpdate)
	order.Status = req.Status

	if req.Status == OrderDelivered {
		now := time.Now()
		order.DeliveredAt = &now
		// Confirm payment
		if order.PaymentInfo.Method == PaymentCash {
			order.PaymentInfo.Status = PaymentPaid
			order.PaymentInfo.PaymentTime = &now
		}
	}

	if err := updateOrder(order); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Order update error"})
		return
	}

	// Real-time update
	sendOrderUpdate(orderID, req.Status, "Order status updated")

	// Send Telegram message to user
	go func() {
		if userDB, err := getUserByNumber(order.UserNumber); err == nil && userDB.TgID != nil {
			userMessage := fmt.Sprintf("📋 Order: %s\n📍 Status: %s", order.OrderID, string(req.Status))
			if err := sendTelegramMessageToUser(*userDB.TgID, userMessage); err != nil {
				log.Printf("Telegram user message error: %v", err)
			} else {
				log.Printf("Telegram user message sent: %s", order.OrderID)
			}
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"message": "Order status updated",
		"order":   order,
	})
}

// ========== SEARCH HANDLER ==========

func searchHandler(c *gin.Context) {
	query := c.Query("q")
	category := c.Query("category")
	lang := getUserLanguage(c.Request.Header)
	minPrice, _ := strconv.Atoi(c.Query("min_price"))
	maxPrice, _ := strconv.Atoi(c.Query("max_price"))
	minRating, _ := strconv.ParseFloat(c.Query("min_rating"), 64)

	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query parameter required"})
		return
	}

	// Check if admin or regular user
	isAdmin := false
	if userInterface, exists := c.Get("user"); exists {
		user := userInterface.(*Claims)
		isAdmin = (user.Role == "admin")
	}

	foods, err := getAllLocalizedFoods(lang, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Data fetch error"})
		return
	}

	// Search
	searchLower := strings.ToLower(query)
	var results []*Food

	for _, food := range foods {
		// Search in name, description and ingredients
		if strings.Contains(strings.ToLower(food.Name), searchLower) ||
			strings.Contains(strings.ToLower(food.Description), searchLower) {
			results = append(results, food)
			continue
		}

		// Search in ingredients
		if food.Ingredients != nil {
			if ingredientsList, ok := food.Ingredients[lang]; ok {
				for _, ingredient := range ingredientsList {
					if strings.Contains(strings.ToLower(ingredient), searchLower) {
						results = append(results, food)
						break
					}
				}
			} else if ingredientsList, ok := food.Ingredients["uz"]; ok {
				for _, ingredient := range ingredientsList {
					if strings.Contains(strings.ToLower(ingredient), searchLower) {
						results = append(results, food)
						break
					}
				}
			}
		}
	}

	// Filters
	if category != "" {
		filtered := []*Food{}
		for _, food := range results {
			if food.Category == category {
				filtered = append(filtered, food)
			}
		}
		results = filtered
	}

	if minPrice > 0 || maxPrice > 0 {
		filtered := []*Food{}
		for _, food := range results {
			if (minPrice == 0 || food.Price >= minPrice) &&
				(maxPrice == 0 || food.Price <= maxPrice) {
				filtered = append(filtered, food)
			}
		}
		results = filtered
	}

	if minRating > 0 {
		filtered := []*Food{}
		for _, food := range results {
			if food.Rating >= minRating {
				filtered = append(filtered, food)
			}
		}
		results = filtered
	}

	// Make ImageURL full URL
	hostURL := getHostURL(c)
	for _, food := range results {
		if food.ImageURL != "" && !strings.HasPrefix(food.ImageURL, "http") {
			food.ImageURL = hostURL + food.ImageURL
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"query":    query,
		"language": lang,
		"results":  results,
		"total":    len(results),
		"filters": gin.H{
			"category":   category,
			"min_price":  minPrice,
			"max_price":  maxPrice,
			"min_rating": minRating,
		},
	})
}

// ========== STATISTICS HANDLER ==========

func getStatisticsHandler(c *gin.Context) {
	// Order statistics
	var totalOrders, pendingOrders, completedOrders, cancelledOrders, totalRevenue int
	var todayOrders, todayRevenue int

	// General statistics
	db.QueryRow("SELECT COUNT(*) FROM orders").Scan(&totalOrders)
	db.QueryRow("SELECT COUNT(*) FROM orders WHERE status IN ('pending', 'confirmed', 'preparing')").Scan(&pendingOrders)
	db.QueryRow("SELECT COUNT(*) FROM orders WHERE status = 'delivered'").Scan(&completedOrders)
	db.QueryRow("SELECT COUNT(*) FROM orders WHERE status = 'cancelled'").Scan(&cancelledOrders)
	db.QueryRow("SELECT COALESCE(SUM(total_price), 0) FROM orders WHERE status = 'delivered'").Scan(&totalRevenue)

	// Today's statistics
	today := time.Now().Format("2006-01-02")
	db.QueryRow("SELECT COUNT(*) FROM orders WHERE DATE(order_time) = $1", today).Scan(&todayOrders)
	db.QueryRow("SELECT COALESCE(SUM(total_price), 0) FROM orders WHERE status = 'delivered' AND DATE(order_time) = $1", today).Scan(&todayRevenue)

	// Food statistics
	var totalFoods, popularFoods int
	db.QueryRow("SELECT COUNT(*) FROM foods").Scan(&totalFoods)
	db.QueryRow("SELECT COUNT(*) FROM foods WHERE is_popular = true OR rating >= 4.0").Scan(&popularFoods)

	// User statistics
	var totalUsers int
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&totalUsers)

	c.JSON(http.StatusOK, gin.H{
		"total_orders":     totalOrders,
		"pending_orders":   pendingOrders,
		"completed_orders": completedOrders,
		"cancelled_orders": cancelledOrders,
		"total_revenue":    totalRevenue,
		"today_orders":     todayOrders,
		"today_revenue":    todayRevenue,
		"total_foods":      totalFoods,
		"total_users":      totalUsers,
		"popular_foods":    popularFoods,
	})
}

// ========== INITIALIZATION ==========

func initializeTestData() error {
	// Admin user
	adminUser := &User{
		ID:        generateID("user"),
		Number:    "770451117",
		Password:  hashPassword("samandar"),
		Role:      "admin",
		FullName:  "Samandar Admin",
		Email:     stringPtr("admin@restaurant.uz"),
		CreatedAt: time.Now(),
		IsActive:  true,
		TgID:      int64Ptr(1713329317),
		Language:  "uz",
	}

	// Check if user exists
	existingUser, err := getUserByNumber(adminUser.Number)
	if err == sql.ErrNoRows {
		if err := createUser(adminUser); err != nil {
			log.Printf("Admin user creation error: %v", err)
		} else {
			log.Println("✅ Admin user created")
		}
	} else if err != nil {
		log.Printf("User check error: %v", err)
	} else {
		log.Printf("✅ Admin user exists: %s", existingUser.FullName)
	}

	// Test user
	testUser := &User{
		ID:        generateID("user"),
		Number:    "998901234567",
		Password:  hashPassword("user123"),
		Role:      "user",
		FullName:  "Test User",
		Email:     stringPtr("user@test.uz"),
		CreatedAt: time.Now(),
		IsActive:  true,
		TgID:      int64Ptr(1066137436),
		Language:  "uz",
	}

	existingTestUser, err := getUserByNumber(testUser.Number)
	if err == sql.ErrNoRows {
		if err := createUser(testUser); err != nil {
			log.Printf("Test user creation error: %v", err)
		} else {
			log.Println("✅ Test user created")
		}
	} else if err != nil {
		log.Printf("Test user check error: %v", err)
	} else {
		log.Printf("✅ Test user exists: %s", existingTestUser.FullName)
	}

	return nil
}

// ========== ROOT HANDLER ==========

func rootHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message":             "Restaurant API - Complete Version with Integer IDs",
		"version":             "6.0.0",
		"supported_languages": []string{"uz", "ru", "en"},
		"features": []string{
			"PostgreSQL Database with BIGSERIAL Integer IDs",
			"Manual ID Insertion Support",
			"File Upload with Food Names",
			"Telegram Bot Integration",
			"GPS Coordinates for Delivery",
			"Stock Management",
			"Real-time Order Tracking",
			"WebSocket Support",
			"Advanced Search",
			"Multi-language Food Support",
			"English Error Messages",
		},
		"endpoints": gin.H{
			"foods":       "/api/foods (PUBLIC)",
			"food_by_id":  "/api/foods/:id (PUBLIC)",
			"categories":  "/api/categories (PUBLIC)",
			"search":      "/api/search (PUBLIC)",
			"upload":      "/api/upload (PUBLIC/AUTH)",
			"orders":      "/api/orders (AUTH)",
			"websocket":   "/ws",
			"statistics":  "/api/admin/statistics (ADMIN)",
			"custom_food": "/api/admin/foods (ADMIN - supports custom_id)",
		},
		"database": gin.H{
			"type":      "PostgreSQL",
			"status":    "connected",
			"id_system": "BIGSERIAL Integer (1, 2, 3, 4...)",
		},
		"integrations": gin.H{
			"telegram_bot":       "enabled",
			"user_notifications": "enabled",
			"file_upload":        "enabled",
			"gps_tracking":       "enabled",
			"manual_id_support":  "enabled",
		},
	})
}

// ========== SETUP ROUTES ==========

func setupRoutes() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// Middleware
	r.Use(corsMiddleware())

	// Static files
	if _, err := os.Stat(UPLOAD_DIR); os.IsNotExist(err) {
		os.MkdirAll(UPLOAD_DIR, 0755)
	}

	r.Static("/uploads", "./"+UPLOAD_DIR)
	r.StaticFile("/favicon.ico", "./favicon.ico")

	// WebSocket endpoint
	r.GET("/ws", handleWebSocket)

	// Root endpoint
	r.GET("/", rootHandler)

	// API group
	api := r.Group("/api")

	// PUBLIC ENDPOINTS
	public := api.Group("/")
	{
		// Categories
		public.GET("/categories", getCategories)

		// Foods (public - only available foods)
		public.GET("/foods", optionalAuthMiddleware(), getAllFoodsHandler)
		public.GET("/foods/:food_id", getFoodHandler)

		// Search
		public.GET("/search", optionalAuthMiddleware(), searchHandler)

		// File uploads (public but can be authenticated)
		public.POST("/upload", optionalAuthMiddleware(), uploadFile)
	}

	// Authentication endpoints
	auth := api.Group("/")
	{
		auth.POST("/register", register)
		auth.POST("/login", login)
	}

	// Protected endpoints
	protected := api.Group("/")
	protected.Use(authMiddleware())
	{
		// Profile
		protected.GET("/profile", getProfile)

		// Orders
		protected.POST("/orders", createOrderHandler)
		protected.GET("/orders", getOrdersHandler)
		protected.GET("/orders/:order_id", getOrderHandler)
	}

	// Admin endpoints
	admin := protected.Group("/admin")
	admin.Use(adminMiddleware())
	{
		// Food management
		admin.POST("/foods", createFoodHandler) // Supports custom_id
		admin.PUT("/foods/:food_id", updateFoodHandler)
		admin.DELETE("/foods/:food_id", deleteFoodHandler)

		// Order management
		admin.PUT("/orders/:order_id/status", updateOrderStatusHandler)

		// Statistics
		admin.GET("/statistics", getStatisticsHandler)

		// Manual ID support
		admin.POST("/update-sequence", func(c *gin.Context) {
			if err := updateSequenceAfterManualInsert(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "Sequence updated successfully"})
		})
	}

	return r
}

// ========== MAIN FUNCTION ==========

func main() {
	// Database initialization
	if err := initDatabase(); err != nil {
		log.Fatalf("❌ Database error: %v", err)
	}

	// Test data initialization
	if err := initializeTestData(); err != nil {
		log.Printf("⚠️ Test data creation error: %v", err)
	}

	// WebSocket handler
	go func() {
		for {
			select {
			case msg := <-broadcast:
				for client := range clients {
					if err := client.WriteMessage(websocket.TextMessage, msg); err != nil {
						log.Printf("WebSocket error: %v", err)
						client.Close()
						delete(clients, client)
					}
				}
			}
		}
	}()

	// Server setup
	r := setupRoutes()

	// Server port
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Restaurant API - Complete Version with Integer IDs:")
	log.Printf("📍 Server: http://localhost:%s", port)
	log.Printf("🔗 WebSocket: ws://localhost:%s/ws", port)
	log.Printf("📚 API Docs: http://localhost:%s/", port)
	log.Printf("🍽️ Public Foods: http://localhost:%s/api/foods", port)
	log.Printf("🔍 Search: http://localhost:%s/api/search", port)
	log.Printf("📤 File Upload: http://localhost:%s/api/upload", port)
	log.Printf("📊 Admin Stats: http://localhost:%s/api/admin/statistics", port)
	log.Printf("🖼️ Static Files: http://localhost:%s/uploads/", port)
	log.Printf("🔢 ID System: BIGSERIAL Integer (1, 2, 3, 4...)")
	log.Printf("🎯 Manual ID: POST /api/admin/foods with custom_id")
	log.Printf("🗄️ Database: PostgreSQL")
	log.Printf("🤖 Telegram Bot: Admin + User Notifications")
	log.Printf("📍 GPS: Delivery Coordinates Support")
	log.Printf("👁️ Visibility: Only available foods (isThere=true, stock>0)")
	log.Printf("🌐 Languages: Foods support UZ/RU/EN, Errors in English")

	log.Fatal(r.Run(":" + port))
}
