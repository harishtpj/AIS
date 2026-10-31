
# Documentation for the AIS configuration API.
#
# AIS provides a Ruby DSL for configuring gateway services, routes,
# authentication, policies, and the gateway listener.
#
# Configuration methods such as `service`, `route`, and `bind` are
# available inside the block passed to {AIS.configure}.
#
# @example Basic configuration
#   AIS.configure do
#     service :orders, 'http://localhost:9001' do
#       auth :sessions
#       auth_path '/login'
#     end
#
#     route '/orders', :orders
#     bind '0.0.0.0:8080'
#   end
#
# @example Unified authentication
#   AIS.configure do
#     unified_auth true
#
#     service :orders, 'http://localhost:9001' do
#       auth :sessions
#       auth_path '/login'
#     end
#
#     route '/orders', :orders
#     bind '0.0.0.0:8080'
#   end
#
# @see AISPolicyBuilder
class AIS
  # Evaluates the configuration block in a new AIS configuration context.
  #
  # @yield Configuration block
  # @return [AIS] configured AIS instance
  def self.configure(&block); end

  # Registers a backend service.
  #
  # @param name [Symbol, String] service identifier
  # @param url [String] backend URL
  # @yield Service configuration block
  def service(name, url, &block); end

  # Enables or disables unified authentication.
  #
  # @param enabled [Boolean] whether unified authentication is enabled
  def unified_auth(enabled); end

  # Maps an incoming route prefix to a registered service.
  #
  # @param path [String] incoming route prefix
  # @param service [Symbol] registered service identifier
  def route(path, service); end

  # Sets the gateway listening address.
  #
  # @param address [String] host and port
  def bind(address); end

  # Registers a policy.
  #
  # @param name [String] policy name
  # @yield Policy definition block
  def policy(name, &block); end

  # Returns the current Unix timestamp in seconds.
  #
  # @return [Integer]
  def self.current_time_epoch; end
end

# AIS-specific Hash extensions used inside service configuration blocks.
#
# These methods are added to Hash by the embedded scripting runtime.
# They configure authentication options for an individual service.
#
# @example
#   service :orders, 'http://localhost:9001' do
#     auth :sessions
#     auth_path '/login'
#   end
class Hash

  # Sets the authentication strategy for a service.
  #
  # The strategy is represented by a Symbol.
  #
  # @param strategy [Symbol] authentication strategy
  # @option strategy [Symbol] :sessions backend session authentication
  # @option strategy [Symbol] :opq_tok opaque-token authentication
  # @return [Hash] the service configuration hash
  #
  # @example
  #   auth :sessions
  #
  # @!method auth(strategy)


  # Sets the backend authentication endpoint for a service.
  #
  # @param path [String] backend login endpoint path
  # @return [Hash] the service configuration hash
  #
  # @example
  #   auth_path '/login'
  #
  # @!method auth_path(path)

end
