AIS.configure do
  unified_auth true

  service :orders, 'http://localhost:9001' do
    auth :sessions
    auth_path '/login'
  end

  service :inventory, 'http://localhost:9002' do
    auth :opq_tok
    auth_path '/login'
  end

  route '/orders', :orders
  route '/inv', :inventory

  bind '0.0.0.0:8080'
end
