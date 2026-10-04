AIS.configure do
    service :users, 'http://localhost:9001'
    service :orders, 'http://localhost:9002'

    route '/users', :users
    route '/orders', :orders

    bind "0.0.0.0:8080"
end