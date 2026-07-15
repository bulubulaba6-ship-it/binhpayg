import psycopg2
import json
import yaml

dsn = "postgres://avnadmin:AVNS_s4t4FBCqjI9lXtwYTpT@pg-13bba9b2-maiphuocanhtai21032005-bd89.j.aivencloud.com:20938/defaultdb?sslmode=require"
conn = psycopg2.connect(dsn)
cur = conn.cursor()

with open('config.yaml', 'r') as f:
    data = yaml.safe_load(f)

cur.execute("UPDATE auth_store SET content = %s::jsonb, updated_at = NOW() WHERE id = 'config'", (json.dumps(data),))
conn.commit()
print("Config synced to Postgres!")
