# Restaurant API - Go Implementation

Bu loyiha Python FastAPI dan Go tilida Gin framework yordamida qayta yozilgan restaurant API hisoblanadi. Ko'p tilli qo'llab-quvvatlash (O'zbek, Rus, Ingliz tillari) bilan.

## Xususiyatlar

- 🌐 Ko'p tilli qo'llab-quvvatlash (O'zbek, Rus, Ingliz)
- 🔐 JWT autentifikatsiya
- 🍽️ Ovqatlar boshqaruvi
- 📦 Buyurtmalar tizimi
- ⭐ Sharh va reyting tizimi
- 🔍 Ko'p tilli qidiruv
- 👨‍💼 Admin panel
- 📱 Telegram integratsiyasi
- 🐳 Docker qo'llab-quvvatlashi

## Texnologiyalar

- **Go 1.19+**
- **Gin Framework** - HTTP web framework
- **JWT** - Autentifikatsiya
- **UUID** - Identifikatorlar yaratish
- **Docker** - Konteynerizatsiya

## O'rnatish

### Talablar

- Go 1.19 yoki undan yuqori
- Docker (ixtiyoriy)

### Lokal ishga tushirish

1. **Repository ni clone qiling:**
```bash
git clone <repository-url>
cd restaurant-api
```

2. **Dependencylarni o'rnating:**
```bash
go mod download
```

3. **Ilovani ishga tushiring:**
```bash
go run main.go
```

Ilova http://localhost:8000 da ishlab turadi.

### Docker bilan ishga tushirish

1. **Docker image yarating:**
```bash
docker build -t restaurant-api .
```

2. **Konteyner ishga tushiring:**
```bash
docker run -p 8000:8000 restaurant-api
```

### Docker Compose bilan

```bash
docker-compose up -d
```

## API Endpointlari

### Asosiy

- `GET /` - API haqida ma'lumot
- `GET /api/categories` - Kategoriyalar ro'yxati (ko'p tilli)
- `GET /api/search` - Ko'p tilli qidiruv

### Autentifikatsiya

- `POST /api/register` - Ro'yxatdan o'tish
- `POST /api/login` - Tizimga kirish
- `GET /api/profile` - Foydalanuvchi profili

### Til sozlamalari

- `POST /api/settings/language` - Tilni o'rnatish
- `GET /api/settings/language` - Til sozlamalarini olish

### Ovqatlar

- `GET /api/foods` - Barcha ovqatlar (ko'p tilli)
- `GET /api/foods/{id}` - Bitta ovqat ma'lumotlari
- `POST /api/foods` - Yangi ovqat qo'shish (admin)
- `PUT /api/foods/{id}` - Ovqat yangilash (admin)
- `DELETE /api/foods/{id}` - Ovqat o'chirish (admin)

### Buyurtmalar

- `POST /api/orders` - Yangi buyurtma berish
- `GET /api/orders` - Buyurtmalar ro'yxati
- `GET /api/orders/{id}` - Bitta buyurtma
- `PUT /api/orders/{id}/status` - Buyurtma holatini yangilash (admin)
- `DELETE /api/orders/{id}` - Buyurtmani bekor qilish

### Sharhlar

- `POST /api/reviews` - Sharh qoldirish
- `GET /api/foods/{id}/reviews` - Ovqat sharhlari

## Test foydalanuvchilar

### Admin
- **Telefon:** 770451117
- **Parol:** samandar
- **Rol:** admin

### Oddiy foydalanuvchi
- **Telefon:** 998901234567
- **Parol:** user123
- **Rol:** user

## Ko'p tilli qo'llab-quvvatlash

API 3 ta tilni qo'llab-quvvatlaydi:

1. **O'zbek (uz)** - asosiy til
2. **Rus (ru)**
3. **Ingliz (en)**

Til o'rnatish:
```bash
curl -X POST http://localhost:8000/api/settings/language \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"language": "uz"}'
```

## Request misollari

### Ro'yxatdan o'tish

```bash
curl -X POST http://localhost:8000/api/register \
  -H "Content-Type: application/json" \
  -d '{
    "number": "998123456789",
    "password": "mypassword",
    "full_name": "Test User",
    "language": "uz"
  }'
```

### Tizimga kirish

```bash
curl -X POST http://localhost:8000/api/login \
  -H "Content-Type: application/json" \
  -d '{
    "number": "998123456789",
    "password": "mypassword"
  }'
```

### Ovqatlar ro'yxati

```bash
curl -X GET http://localhost:8000/api/foods \
  -H "Accept-Language: uz"
```

### Buyurtma berish

```bash
curl -X POST http://localhost:8000/api/orders \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "food_ids": [
      {"amur_1": 2},
      {"amur_3": 1}
    ],
    "to_give": {
      "delivery": "Toshkent, Chilonzor tumani"
    },
    "payment_method": "cash",
    "special_instructions": "Iltimos tezroq yetkazing"
  }'
```

### Ko'p tilli qidiruv

```bash
curl -X GET "http://localhost:8000/api/search?q=shashlik&category=shashlik" \
  -H "Accept-Language: ru"
```

## Ma'lumotlar strukturasi

### Ovqat (Food)
```json
{
  "id": "amur_1",
  "name": "Moloti",
  "description": "Mol go'shtidan shashlik juda ham mazzali qiyma",
  "category": "shashlik",
  "category_name": "Shashlik",
  "price": 23000,
  "isThere": true,
  "imageUrl": "http://localhost:8000/uploads/moloti.png",
  "ingredients": ["Mol go'shti", "Piyoz", "Ziravorlar"],
  "allergens": [],
  "rating": 4.5,
  "review_count": 15,
  "preparation_time": 20
}
```

### Buyurtma (Order)
```json
{
  "order_id": "2024-01-15-1",
  "user_number": "998123456789",
  "user_name": "Test User",
  "foods": [...],
  "total_price": 46000,
  "order_time": "2024-01-15T10:30:00Z",
  "delivery_type": "delivery",
  "delivery_info": {
    "type": "delivery",
    "location": "Toshkent, Chilonzor tumani"
  },
  "status": "pending",
  "payment_info": {
    "method": "cash",
    "status": "pending",
    "amount": 46000
  },
  "estimated_time": 25
}
```

## Xatoliklar bilan ishlash

API standart HTTP status kodlarini ishlatadi:

- `200` - Muvaffaqiyatli
- `201` - Yaratildi
- `400` - Noto'g'ri so'rov
- `401` - Avtorizatsiya talab etiladi
- `403` - Taqiqlangan
- `404` - Topilmadi
- `500` - Server xatosi

Xatolik javoblari:
```json
{
  "error": "Xatolik tavsifi",
  "message": "Ko'p tilli xabar",
  "language": "uz"
}
```

## Kelajakdagi rejalar

- [ ] PostgreSQL ma'lumotlar bazasi
- [ ] Redis keshi
- [ ] WebSocket real-time yangilanishlar
- [ ] File upload API
- [ ] Email bildirishnomalar
- [ ] Telegram bot integratsiyasi
- [ ] Payment gateway integratsiyasi
- [ ] API versioning
- [ ] Rate limiting
- [ ] API documentation (Swagger)
- [ ] Tests
- [ ] Monitoring va logging

## Hissa qo'shish

1. Fork qiling
2. Feature branch yarating (`git checkout -b feature/yangi-xususiyat`)
3. Commit qiling (`git commit -am 'Yangi xususiyat qo'shildi'`)
4. Push qiling (`git push origin feature/yangi-xususiyat`)
5. Pull Request yarating

## Litsenziya

MIT License

## Muallif

Samandar - Restaurant API Go Implementation

---

**Eslatma:** Bu loyiha test maqsadida yaratilgan va production muhitida ishlatishdan oldin qo'shimcha xavfsizlik choralarini ko'rish tavsiya etiladi.