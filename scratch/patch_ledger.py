import psycopg2
import json
import datetime

dsn = "postgres://avnadmin:AVNS_s4t4FBCqjI9lXtwYTpT@pg-13bba9b2-maiphuocanhtai21032005-bd89.j.aivencloud.com:20938/defaultdb?sslmode=require"
target_key = "fink_test_payg_key"

try:
    conn = psycopg2.connect(dsn)
    cur = conn.cursor()
    
    cur.execute("SELECT content FROM auth_store WHERE id = 'ledger'")
    row = cur.fetchone()
    if row:
        val = row[0]
        ledger_data = json.loads(val) if isinstance(val, str) else val
        
        # Inject the key history
        if target_key not in ledger_data:
            ledger_data[target_key] = {}
        
        ledger_data[target_key]["CreditsConsumed"] = 0.0
        # Initialize other required fields if they don't exist
        if "Timestamp" not in ledger_data[target_key]:
            ledger_data[target_key]["Timestamp"] = datetime.datetime.utcnow().isoformat() + "Z"
        if "CreditsPurchased" not in ledger_data[target_key] or ledger_data[target_key]["CreditsPurchased"] == 0.0:
            ledger_data[target_key]["CreditsPurchased"] = 100000.0
            
        print(f"Updating ledger for {target_key}:", ledger_data[target_key])
        
        # Save back to database
        updated_json = json.dumps(ledger_data)
        cur.execute("""
            UPDATE auth_store
            SET content = %s::jsonb, updated_at = NOW()
            WHERE id = 'ledger'
        """, (updated_json,))
        
        conn.commit()
        print("Successfully updated Postgres ledger.")
    else:
        print("No ledger found!")
        
    cur.close()
    conn.close()
except Exception as e:
    print("Database error:", e)
