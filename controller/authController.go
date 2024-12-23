package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	models "github.com/ChekoutGobiz/BackendChekout/model"
	"github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

var client *mongo.Client

func init() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Ambil MONGODB_URI dari environment
	mongoURI := os.Getenv("MONGODB_URI")

	// Opsi koneksi MongoDB
	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err = mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatal(err)
	}

	// Cek koneksi MongoDB
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}

	fmt.Println("MongoDB connection established successfully!")
}

// Register function for user registration
func Register(c *fiber.Ctx) error {
	var user models.User
	if err := c.BodyParser(&user); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid data"})
	}

	// Hash the password
    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Error hashing password"})
    }
    user.Password = string(hashedPassword) // Pastikan hashedPassword disimpan di database
    

	// Generate unique ObjectID
	user.ID = primitive.NewObjectID()

	// Insert user to database
	collection := client.Database("jajankuy").Collection("users")
	_, err = collection.InsertOne(context.TODO(), user)
	if err != nil {
		log.Println("Error inserting user:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Error saving user"})
	}

	// Remove password from response for security
	user.Password = ""

	// Return the created user in response
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "User successfully registered",
		"user": fiber.Map{
			"_id":   user.ID.Hex(),
			"name":  user.Name,
			"email": user.Email,
		},
	})
}

func Login(c *fiber.Ctx) error {
	var loginData models.User
	var storedUser models.User

	// Parse the login data from the request body
	if err := json.Unmarshal(c.Body(), &loginData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid data"})
	}

	// Query database to find the user by email
	collection := client.Database("jajankuy").Collection("users")
	err := collection.FindOne(context.TODO(), bson.M{"email": loginData.Email}).Decode(&storedUser)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not found"})
	}

	// Debugging: Check passwords
	log.Printf("Login password: %s", loginData.Password)
	log.Printf("Stored hashed password: %s", storedUser.Password)

	// Compare the hashed password with the provided password
    if err := bcrypt.CompareHashAndPassword([]byte(storedUser.Password), []byte(loginData.Password)); err != nil {
        return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid password"})
    }
    

	// Generate JWT Token
	token, err := generateJWT(storedUser.Email, storedUser.ID.Hex()) // Convert ObjectID to string
	if err != nil {
		log.Println("Error generating JWT token:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Error generating token"})
	}

	// Log the generated token
	log.Println("Generated JWT token:", token)

	// Return the generated token with user data
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"user":  storedUser,
		"token": token, // Add "Bearer" prefix to the token
	})
}

func Logout(c *fiber.Ctx) error {
    // Mengambil token dari header Authorization
    token := c.Get("Authorization")

    // Jika token kosong, kembalikan respons error
    if token == "" {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "error": "No token provided",
        })
    }

    // Hapus prefix "Bearer " jika ada
    token = token[len("Bearer "):]

    // Simpan token yang dicabut ke dalam database MongoDB (blacklist token)
    collection := client.Database("jajankuy").Collection("blacklisted_tokens")
    blacklistedToken := models.BlacklistedToken{
        Token:     token,
        CreatedAt: time.Now().Unix(),
    }

    _, err := collection.InsertOne(context.TODO(), blacklistedToken)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
            "error": "Error blacklisting token",
        })
    }

    // Menyampaikan bahwa logout berhasil dan token harus dihapus dari client
    return c.Status(fiber.StatusOK).JSON(fiber.Map{
        "message": "Successfully logged out. Token has been blacklisted.",
    })
}

// generateJWT generates a JWT token for the given email and user ID
func generateJWT(email string, userID string) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)

	// Set claims for the token
	claims["email"] = email
	claims["user_id"] = userID                            // Pass the user ID as a string
	claims["exp"] = time.Now().Add(time.Hour * 24).Unix() // Expiration in 24 hours

	// Get JWT_SECRET from .env
	jwtSecret := os.Getenv("JWT_SECRET")

	// Create the token
	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		log.Println("Error creating JWT:", err)
		return "", err
	}

	return tokenString, nil
}