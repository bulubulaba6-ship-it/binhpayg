package main

import (
	"bytes"
	"database/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"log"
	"net/http"
)

func main() {
	dsn := "postgres://avnadmin:AVNS_gMMqDMXI4MLgS0D6m01@pg-34ab11a6-zok210305-4ca5.g.aivencloud.com:10436/defaultdb?sslmode=require"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("INSERT INTO payment_orders (order_code, email, plan, amount) VALUES (1780303766020, 'maiphuocanhtai21032005@gmail.com', 'pro', 350000)")
	if err != nil {
		log.Printf("Insert error: %v", err) // might already exist, that's fine
	} else {
		log.Println("Inserted mock order successfully")
	}

	payload := `{"code":"00","desc":"success","success":true,"data":{"orderCode":1780303766020,"amount":350000,"description":"PRO order","accountNumber":"12345678","reference":"TF230204212323","transactionDateTime":"2023-02-04 18:25:00","currency":"VND","paymentLinkId":"124c33293c43417ab7879e14c8d9eb18","code":"00","desc":"Thành công","counterAccountBankId":"","counterAccountBankName":"","counterAccountName":"","counterAccountNumber":"","virtualAccountName":"","virtualAccountNumber":""},"signature":"977b583c494f8db9604c57a01cc695a513cd28c43d78fd6cd2303936902e3f4c"}`

	req, err := http.NewRequest("POST", "http://localhost:8317/payos-webhook", bytes.NewBuffer([]byte(payload)))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	log.Printf("Webhook response status: %d", resp.StatusCode)
}
