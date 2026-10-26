AIS.configure do
    service :orders, 'http://localhost:9001'
    service :inventory, 'http://localhost:9002'

    route '/orders', :orders
    route '/inv', :inventory

    bind "0.0.0.0:8080"
end
