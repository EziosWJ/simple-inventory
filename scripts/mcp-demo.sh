#!/usr/bin/env bash
# Create synthetic, disposable SQLite fixtures; never accepts a database path.
set -euo pipefail
project_dir=$(cd "$(dirname "$0")/.." && pwd)
demo_dir=$(mktemp -d /tmp/simple-inventory-mcp-demo.XXXXXX)
chmod 700 "$demo_dir"
export APP_ENV=test APP_CONFIG_PROFILE=sqlite
export APP_DATABASE__DRIVER=sqlite APP_DATABASE__URL="$demo_dir/mcp-demo.db"
export APP_DATABASE__USERNAME='' APP_DATABASE__PASSWORD=''
export APP_JWT__SECRET
APP_JWT__SECRET=$(openssl rand -hex 32)
export APP_FILE__STORAGE_ROOT="$demo_dir/uploads"
export APP_HTTP__ADDRESS=0.0.0.0:18300 APP_MCP__ENABLED=true
export APP_MCP__ALLOWED_HOSTS="127.0.0.1:18300,localhost:18300${MCP_DEMO_HOST:+,$MCP_DEMO_HOST:18300}"
export APP_MCP__ALLOWED_ORIGINS=''
export GOCACHE="$project_dir/.task/go-build"
# Persist only local environment settings (no MCP token); the directory is private.
(umask 077; for setting in APP_ENV APP_CONFIG_PROFILE APP_DATABASE__DRIVER APP_DATABASE__URL APP_DATABASE__USERNAME APP_DATABASE__PASSWORD APP_JWT__SECRET APP_FILE__STORAGE_ROOT APP_HTTP__ADDRESS APP_MCP__ENABLED APP_MCP__ALLOWED_HOSTS APP_MCP__ALLOWED_ORIGINS GOCACHE; do printf 'export %s=%q\n' "$setting" "${!setting}"; done) > "$demo_dir/env.sh"
cd "$project_dir/server"
go run ./cmd/migrate up --kind all
# These are synthetic read fixtures, not a business import/posting workflow.
python3 - "$APP_DATABASE__URL" <<'PY'
import sqlite3,sys
with sqlite3.connect(sys.argv[1]) as db:
    db.execute('PRAGMA foreign_keys=ON')
    db.execute("INSERT INTO sys_user(username,nickname,password,status) VALUES ('father-mcp-test','父亲隔离测试身份','!',1)")
    user=db.execute("SELECT id FROM sys_user WHERE username='father-mcp-test'").fetchone()[0]
    db.execute("INSERT INTO sys_role(role_name,role_code,status) VALUES ('隔离只读试用','mcp-demo',1)")
    role=db.execute("SELECT id FROM sys_role WHERE role_code='mcp-demo'").fetchone()[0]
    db.execute('INSERT INTO sys_user_role(user_id,role_id) VALUES (?,?)',(user,role))
    for code in ['business:product','business:partner','business:inventory-balance','business:partner-balance']:
        db.execute('INSERT INTO sys_role_menu(role_id,menu_id) SELECT ?,id FROM sys_menu WHERE permission_code=?',(role,code))
    for code,name,kind,unit,model in [('DEMO-G1','同名硒鼓','GOODS','支','A型'),('DEMO-G2','同名硒鼓','GOODS','支','B型'),('DEMO-S','安装服务','SERVICE','次','')]:
        db.execute('INSERT INTO product(code,name,type,unit,model) VALUES (?,?,?,?,?)',(code,name,kind,unit,model))
    goods=db.execute("SELECT id FROM product WHERE code='DEMO-G1'").fetchone()[0]
    db.execute('INSERT INTO inventory_balance(product_id,quantity_milli) VALUES (?,12500)',(goods,))
    for code,name in [('DEMO-P1','双向往来样例'),('DEMO-P2','无往来样例')]:
        db.execute("INSERT INTO partner(code,name,type,is_customer,is_supplier) VALUES (?,?,'COMPANY',1,1)",(code,name))
    partner=db.execute("SELECT id FROM partner WHERE code='DEMO-P1'").fetchone()[0]
    db.execute("INSERT INTO partner_balance(partner_id,direction,amount_cents,entry_count) VALUES (?,'CUSTOMER',12345,1)",(partner,))
    db.execute("INSERT INTO partner_balance(partner_id,direction,amount_cents,entry_count) VALUES (?,'SUPPLIER',-6789,1)",(partner,))
    print('隔离身份 userId =',user)
print('样例：DEMO-G1=12.5支；DEMO-G2=0；DEMO-S=服务；DEMO-P1客户欠123.45、供应商应退67.89；DEMO-P2无记录。')
PY
printf '\n隔离环境：%s\n先 source 此文件，再在 server/ 运行 mcp-token issue 和 go run ./cmd/api。\n' "$demo_dir/env.sh"
