import psycopg2

dsn = "postgres://avnadmin:AVNS_s4t4FBCqjI9lXtwYTpT@pg-13bba9b2-maiphuocanhtai21032005-bd89.j.aivencloud.com:20938/defaultdb?sslmode=require"

try:
    conn = psycopg2.connect(dsn)
    cur = conn.cursor()
    
    cur.execute("SELECT content FROM config_store WHERE id = 'config'")
    content = cur.fetchone()[0]
    
    lines = content.split('\n')
    for j in range(0, min(30, len(lines))):
        print(f"{j+1}: {lines[j]}")
                
    cur.close()
    conn.close()
except Exception as e:
    print("Database error:", e)
