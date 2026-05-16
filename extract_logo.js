const fs = require('fs');
const data = fs.readFileSync('c:/gitroot/backup/static/management.html', 'utf8');
const matches = data.match(/data:image\/[^;]+;base64,[a-zA-Z0-9+/=]+/g);
if (matches) { matches.forEach(m => console.log(m.substring(0, 50) + '...')); }
const urlMatches = data.match(/[^]+\.(png|jpg|svg|ico)/g) || data.match(/'[^']+\.(png|jpg|svg|ico)'/g) || data.match(/[^]+\.(png|jpg|svg|ico)/g);
if (urlMatches) console.log('URLs:', urlMatches);
const cpamcMatch = data.match(/x\s*=\s*(['].*?['])/);
if (cpamcMatch) console.log('x definition:', cpamcMatch[0].substring(0, 100));
