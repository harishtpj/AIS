AIS.configure do
  unified_auth true

  service :orders, 'http://localhost:9001', auth: :sessions
  service :inventory, 'http://localhost:9002', auth: :opq_tok

  route '/orders', :orders
  route '/inv', :inventory

  bind '0.0.0.0:8080'
end
