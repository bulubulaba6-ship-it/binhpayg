const crypto = require('crypto');

const CHECKSUM_KEY = process.env.PAYOS_CHECKSUM_KEY || "4b79b69e12f73459af9559fcc188fd3197aa1e8eb72507897c2e7883f3ecdd1f";

const orderCode = Date.now();

// Example webhook data payload
const body = {
  "code": "00",
  "desc": "success",
  "success": true,
  "data": {
    "orderCode": orderCode,
    "amount": 350000,
    "description": "PRO order",
    "accountNumber": "12345678",
    "reference": "TF230204212323",
    "transactionDateTime": "2023-02-04 18:25:00",
    "currency": "VND",
    "paymentLinkId": "124c33293c43417ab7879e14c8d9eb18",
    "code": "00",
    "desc": "Thành công",
    "counterAccountBankId": "",
    "counterAccountBankName": "",
    "counterAccountName": "",
    "counterAccountNumber": "",
    "virtualAccountName": "",
    "virtualAccountNumber": ""
  }
};

// 1. Sort the object by key alphabetically
const sortedDataByKey = Object.keys(body.data).sort().reduce((obj, key) => {
  obj[key] = body.data[key];
  return obj;
}, {});

// 2. Convert to query string format per payOS requirements
const dataQueryStr = Object.keys(sortedDataByKey)
  .filter(key => sortedDataByKey[key] !== undefined)
  .map(key => {
    let value = sortedDataByKey[key];
    if (Array.isArray(value)) value = JSON.stringify(value);
    if ([null, undefined, 'undefined', 'null'].includes(value)) value = '';
    return `${key}=${value}`;
  }).join('&');

// 3. Create HMAC SHA256 Signature
const expectedSig = crypto.createHmac('sha256', CHECKSUM_KEY).update(dataQueryStr).digest('hex');
body.signature = expectedSig;

// We need to insert the mock order into Postgres first, then trigger this.
// I'll print the SQL to run, and the payload.
console.log(`INSERT INTO payment_orders (order_code, email, plan, amount) VALUES (${orderCode}, 'maiphuocanhtai21032005@gmail.com', 'pro', 350000);`);
console.log(JSON.stringify(body));
