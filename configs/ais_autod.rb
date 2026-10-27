# Automatic Discovery script. Run with appropriate ENV keys (ref. .env.example)
AIS.configure do
  unified_auth true

  route '/orders', :orders
  route '/inv', :inventory

  bind '0.0.0.0:8080'
end
