#!/usr/bin/env fish

set -l ais_url (string trim --right --chars=/ (set -q AIS_URL[1]; and echo $AIS_URL; or echo http://127.0.0.1:8080))
set -l session_backend (string trim --right --chars=/ (set -q SESSION_BACKEND_URL[1]; and echo $SESSION_BACKEND_URL; or echo http://127.0.0.1:9003))
set -l public_backend (string trim --right --chars=/ (set -q PUBLIC_BACKEND_URL[1]; and echo $PUBLIC_BACKEND_URL; or echo http://127.0.0.1:9004))
set -g tmp_dir (mktemp -d)
set -g failures 0

function finish --on-event fish_exit
    rm -rf $tmp_dir
end

function check_status
    set -l expected $argv[1]
    set -l url $argv[2]
    set -l output $tmp_dir/response-(random).json
    set -l actual (curl -sS -o $output -w '%{http_code}' $url)
    if test "$actual" != "$expected"
        echo "FAIL $url: expected HTTP $expected, got $actual"
        cat $output
        echo
        set -g failures (math $failures + 1)
        return 1
    end
    echo "PASS HTTP $expected $url"
    return 0
end

function check_json_field
    set -l file $argv[1]
    set -l expression $argv[2]
    if not jq -e "$expression" $file >/dev/null
        echo "FAIL JSON assertion $expression in $file"
        cat $file
        echo
        set -g failures (math $failures + 1)
        return 1
    end
    echo "PASS JSON $expression"
end

echo "Checking direct public backend..."
set -l public_file $tmp_dir/public.json
set -l public_status (curl -sS -o $public_file -w '%{http_code}' $public_backend/items)
if test "$public_status" = "200"
    echo "PASS direct public GET /items"
else
    echo "FAIL direct public GET /items: got $public_status"
    cat $public_file
    set -g failures (math $failures + 1)
end

echo "Checking direct session authentication..."
check_status 401 $session_backend/items

set -l login_file $tmp_dir/login.json
set -l login_status (curl -sS -o $login_file -w '%{http_code}' \
    -H 'Content-Type: application/json' \
    -d '{"username":"alice","password":"password"}' \
    $session_backend/login)
if test "$login_status" = "200"
    echo "PASS direct session login"
else
    echo "FAIL direct session login: got $login_status"
    cat $login_file
    set -g failures (math $failures + 1)
end

echo "Checking AIS public routing..."
set -l ais_public_file $tmp_dir/ais-public.json
set -l ais_public_status (curl -sS -o $ais_public_file -w '%{http_code}' $ais_url/public/items)
if test "$ais_public_status" = "200"
    echo "PASS AIS public route"
else
    echo "FAIL AIS public route: got $ais_public_status"
    cat $ais_public_file
    set -g failures (math $failures + 1)
end

echo "Checking AIS protected route..."
check_status 401 $ais_url/session/items

set -l ais_login_file $tmp_dir/ais-login.json
set -l ais_login_status (curl -sS -o $ais_login_file -w '%{http_code}' \
    -H 'Content-Type: application/json' \
    -d '{"username":"alice","password":"password"}' \
    $ais_url/auth/session-api/login)
if test "$ais_login_status" != "200"
    echo "FAIL AIS login: got $ais_login_status"
    cat $ais_login_file
    set -g failures (math $failures + 1)
else
    echo "PASS AIS login"
    set ais_token (jq -r '.access_token // empty' $ais_login_file)
    if test -z "$ais_token"
        echo "FAIL AIS login response has no access_token"
        set -g failures (math $failures + 1)
    else
        echo "PASS AIS login returned access_token"

        set -l ais_items_file $tmp_dir/ais-items.json
        set -l auth_header (string join '' 'Authorization: Bearer ' $ais_token)
        set -l ais_items_status (curl -sS -o $ais_items_file -w '%{http_code}' \
            -H $auth_header \
            $ais_url/session/items)
        if test "$ais_items_status" = "200"
            echo "PASS AIS authenticated route"
        else
            echo "FAIL AIS authenticated route: got $ais_items_status"
            cat $ais_items_file
            set -g failures (math $failures + 1)
        end

        set -l create_file $tmp_dir/create.json
        set -l create_status (curl -sS -o $create_file -w '%{http_code}' \
            -H $auth_header \
            -H 'Content-Type: application/json' \
            -d '{"name":"Laptop","price":1200}' \
            $ais_url/session/items)
        if test "$create_status" = "200"
            echo "PASS AIS authenticated POST"
            check_json_field $create_file '.id != null and .name == "Laptop"'
        else
            echo "FAIL AIS authenticated POST: got $create_status"
            cat $create_file
            set -g failures (math $failures + 1)
        end
    end
end

if test $failures -gt 0
    echo "$failures smoke test(s) failed"
    exit 1
end

echo "All AIS smoke tests passed"
