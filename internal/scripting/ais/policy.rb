# Ruby-facing policy DSL and request objs for simplification
class AISPolicyLocation
  attr_reader :country, :region, :region_code, :city, :latitude, :longitude,
              :timezone, :source

  def initialize(country, region, region_code, city, latitude, longitude, timezone, source, available)
    @country = country
    @region = region
    @region_code = region_code
    @city = city
    @latitude = latitude
    @longitude = longitude
    @timezone = timezone
    @source = source
    @available = available
  end

  def available?
    @available
  end
end

class AISPolicyTimeInfo
  def initialize(epoch)
    @epoch = epoch.to_i
  end

  def now
    Time.at(@epoch)
  end
end

class AISPolicyRequest
  attr_reader :method, :path, :ip, :location, :timeinfo, :claims

  def initialize(method, path, ip, location, epoch)
    @method = method
    @path = path
    @ip = ip
    @location = location
    @timeinfo = AISPolicyTimeInfo.new(epoch)
    @claims = {}
  end
end

class AISPolicyBuilder
  attr_reader :name, :services, :routes, :evaluator

  def initialize(name)
    @name = name.to_s
    @services = []
    @routes = []
    @evaluator = nil
    @uses_location = false
    @decision = nil
    @current_request = nil
  end

  def for_services(*names)
    @services = names.flatten.map { |name| name.to_s }
  end

  def for_routes(*patterns)
    @routes = patterns.flatten.map { |pattern| pattern.to_s }
  end

  def uses_location
    @uses_location = true
  end

  def location_required?
    @uses_location
  end

  def evaluate(&block)
    raise ArgumentError, "policy '#{@name}' requires an evaluate block" unless block
    @evaluator = block
  end

  def route_to(service)
    @decision = service.to_s
  end

  def __begin_request(request)
    @current_request = request
    @decision = nil
  end

  def __decision
    @decision
  end

  def timeinfo
    @current_request.timeinfo
  end

  def request_info
    @current_request
  end
end
