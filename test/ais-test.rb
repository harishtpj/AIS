AIS.configure do
  unified_auth true

  route '/session', :'session-api'
  route '/public', :'public-api'

  bind '0.0.0.0:8080'
end
