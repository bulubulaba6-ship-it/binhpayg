import psycopg2
import yaml

dsn = "postgres://avnadmin:AVNS_s4t4FBCqjI9lXtwYTpT@pg-13bba9b2-maiphuocanhtai21032005-bd89.j.aivencloud.com:20938/defaultdb?sslmode=require"

try:
    conn = psycopg2.connect(dsn)
    cur = conn.cursor()
    
    print("--- Querying config_store for config ---")
    cur.execute("SELECT content FROM config_store WHERE id = 'config'")
    row = cur.fetchone()
    if row:
        val = row[0]
        config_data = yaml.safe_load(val)
        api_keys = config_data.get("api-keys", [])
        print("Config contains", len(api_keys), "API keys")
        
        target_key = "fink_9e90275986313d820ff099aa58b47e73"
        if target_key in api_keys:
            print(f"Target key {target_key} is IN api-keys list in Postgres config!")
        else:
            print(f"Target key {target_key} NOT in api-keys list in Postgres config!")
            print("First 10 keys in config:")
            for k in api_keys[:10]:
                print("  ", k)
    else:
        print("No config row found in config_store")
        
    cur.close()
    conn.close()
except Exception as e:
    print("Database error:", e)
