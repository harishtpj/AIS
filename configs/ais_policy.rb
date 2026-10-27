AIS.configure do
  unified_auth false

  service :orders, 'http://localhost:9001'
  service :inventory, 'http://localhost:9002'

  route '/orders', :orders
  route '/inv', :inventory

  policy 'Route orders by client geography' do
    for_services :orders, :inventory
    for_routes '/orders'
    uses_location

    evaluate do |request|
      if request.location.available? && request.location.country == 'IN'
        route_to :orders
      else
        route_to :inventory
      end
    end
  end

  policy 'Route inventory by time of day' do
    for_services :orders, :inventory
    for_routes '/inv'

    evaluate do |request|
      now = request.timeinfo.now
      if now.hour >= 9 && now.hour < 10
        route_to :inventory
      else
        route_to :orders
      end
    end
  end

  bind '0.0.0.0:8080'
end
