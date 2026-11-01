AIS.configure do
  unified_auth true

  route '/public', :'public-api'
  route '/session', :'session-api'
  route '/inventory', :inventory
  route '/smart', :'public-api'

  policy 'Select smart backend from request context' do
    for_services :'public-api', :'session-api'
    for_routes '/smart'

    evaluate do |request|
      local_request = request.ip == '127.0.0.1' || request.ip == '::1'
      if local_request && request.method == 'POST'
        route_to :'session-api'
      elsif local_request && request.method == 'GET'
        route_to :'public-api'
      elsif request.timeinfo.now.hour < 12
        route_to :'public-api'
      else
        route_to :'session-api'
      end
    end
  end

  bind '0.0.0.0:8080'
end
