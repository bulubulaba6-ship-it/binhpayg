import psycopg2
import json

dsn = "postgres://avnadmin:AVNS_s4t4FBCqjI9lXtwYTpT@pg-13bba9b2-maiphuocanhtai21032005-bd89.j.aivencloud.com:20938/defaultdb?sslmode=require"

try:
    conn = psycopg2.connect(dsn)
    cur = conn.cursor()
    print("--- Querying usage history for target key ---")
    target_key = "fink_max_3d6be4e0c6f57d845c49b853b246053a"
    
    # Check usage_logs
    try:
        cur.execute("SELECT COUNT(*), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0) FROM usage_logs WHERE api_key = %s", (target_key,))
        row = cur.fetchone()
        print(f"From usage_logs -> Requests: {row[0]}, Input Tokens: {row[1]}, Output Tokens: {row[2]}")
    except Exception as e:
        print("Error querying usage_logs:", e)
        conn.rollback()

    # Check daily_usage_summary
    try:
        cur.execute("SELECT COUNT(*), COALESCE(SUM(total_requests), 0), COALESCE(SUM(total_tokens), 0) FROM daily_usage_summary WHERE api_key = %s", (target_key,))
        row = cur.fetchone()
        print(f"From daily_usage_summary -> Days: {row[0]}, Requests: {row[1]}, Total Tokens: {row[2]}")
    except Exception as e:
        print("Error querying daily_usage_summary:", e)
        conn.rollback()
        
    cur.close()
    conn.close()
except Exception as e:
    print("Database error:", e)
