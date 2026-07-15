Start-Process -FilePath "go" -ArgumentList "run ./cmd/server --config config.yaml" -NoNewWindow -RedirectStandardError "server_err.txt" -RedirectStandardOutput "server_out.txt" -PassThru | Set-Variable -Name proc
Start-Sleep -Seconds 4
$body = '{"model": "gpt-5.5", "messages": [{"role": "user", "content": "Hello!"}], "stream": true}'
curl.exe -s -i -X POST -H "Authorization: Bearer test_fink_pro_1234567890abcdef_test" -H "Content-Type: application/json" -d $body http://127.0.0.1:8317/v1/chat/completions > curl_out.txt 2> curl_err.txt
Start-Sleep -Seconds 1
Stop-Process -InputObject $proc -Force
