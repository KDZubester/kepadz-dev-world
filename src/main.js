import './style.css'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls'
import { GLTFLoader } from 'three/addons/loaders/GLTFLoader.js';
import { RoomEnvironment } from 'three/addons/environments/RoomEnvironment.js';

// Create a scene
const scene = new THREE.Scene();

// Create a camera
  // FOV: 75%
  // Aspect Ratio: window size
  // View Frustrum
    // See up to 0.1 close to you
    // See up to 1000 away from you
const camera = new THREE.PerspectiveCamera( 75, 
                                            window.innerWidth / window.innerHeight,
                                            0.1, 
                                            15
                                          );

// Create a renderer that renders the scene and camera (make the magic happen)
const renderer = new THREE.WebGLRenderer({
  canvas: document.querySelector( '#bg' ),
});

renderer.setPixelRatio( window.devicePixelRatio );
renderer.setSize( window.innerWidth, window.innerHeight );

// Set camera position
// camera.position.setZ( 10 );

// Actually render the scene
renderer.render( scene, camera );

const ambientLIght = new THREE.AmbientLight( 0xffffff, 3 );
scene.add( ambientLIght );

// Make it so that you can move around the scene with your mouse
const controls = new OrbitControls( camera, renderer.domElement );


// Function to add starts to screen
function addStar() {
  const geometry = new THREE.SphereGeometry( 0.25, 24, 24 );
  const material = new THREE.MeshStandardMaterial( { color: 0xffffff } );
  const star = new THREE.Mesh( geometry, material );

  const [x, y, z] = Array( 3 ).fill().map( () => THREE.MathUtils.randFloatSpread( 75 ) ); 

  star.position.set( x, y, z );
  scene.add( star );
}
Array( 200 ).fill().forEach( addStar );


// Instantiate the TextureLoader
const textureLoader = new THREE.TextureLoader();

// Load the texture
textureLoader.load('./art/space.jpg', (texture) => {
    // Apply the texture as the scene background
    scene.background = texture;
}, undefined, (error) => {
    console.error('An error occurred while loading the texture:', error);
});



// // Load a 3D model in glTF format
const assetLoader = new GLTFLoader();

let prince;
assetLoader.load('./art/prince_fox.glb', function(gltf) {
  prince = gltf.scene;
  prince.position.set(3, 0, 0);
  prince.scale.set(0.25, 0.25, 0.25);
  scene.add(prince);

}, undefined, function(error) {
  console.error(error);
});


let rose;
assetLoader.load('./art/rose.glb', function(gltf) {
  rose = gltf.scene;
  rose.position.set(2.25, 0, 5);
  rose.scale.set(0.125, 0.125, 0.125);
  scene.add(rose);

}, undefined, function(error) {
  console.error(error);
});


let baobab;
assetLoader.load('./art/baobab.glb', function(gltf) {
  baobab = gltf.scene;
  baobab.position.set(-2.5, -1, 10);
  baobab.scale.set(0.2, 0.2, 0.2);
  baobab.rotation.y = 45;
  scene.add(baobab);

}, undefined, function(error) {
  console.error(error);
});


let box;
assetLoader.load('./art/box.glb', function(gltf) {
  box = gltf.scene;
  box.position.set(2, 0, 20);
  box.scale.set(0.125, 0.125, 0.125);
  box.rotation.y = 25;
  scene.add(box);

}, undefined, function(error) {
  console.error(error);
});


let sheep;
assetLoader.load('./art/sheep.glb', function(gltf) {
  sheep = gltf.scene;
  sheep.position.set(-2, 0, 30);
  sheep.scale.set(0.25, 0.25, 0.25);
  sheep.rotation.y = 25;
  scene.add(sheep);

}, undefined, function(error) {
  console.error(error);
});

let mixer;
assetLoader.load('./art/plane.glb', function(gltf) {
    const model = gltf.scene;
    model.position.set(7, 0, 38);
    model.scale.set(0.25, 0.25, 0.25);
    scene.add(model);

    mixer = new THREE.AnimationMixer(model);
    const clips = gltf.animations;

    // Play all animations at the same time
    clips.forEach(function(clip) {
        const action = mixer.clipAction(clip);
        action.play();
    });

}, undefined, function(error) {
    console.error(error);
});

const environment = new RoomEnvironment( renderer );
const pmremGenerator = new THREE.PMREMGenerator( renderer );

// scene = new THREE.Scene();
scene.environment = pmremGenerator.fromScene( environment ).texture;
environment.dispose();


// Scroll Animation
function moveCamera() {
  const t = document.body.getBoundingClientRect().top;

  // Prevent the camera from moving out of bounds
  camera.position.z = Math.max(5, t * -0.0075); // Ensure z doesn't go too far back
  camera.position.x = t * -0.0001;
  camera.rotation.y = t * -0.0001;

  // Check the distance between the camera and the prince
  if (prince) {
    const distance = camera.position.distanceTo(prince.position);

    // Set a threshold for when the prince should disappear
    if (distance > 6) {
      prince.visible = false; // Hide the prince
    } else {
      prince.visible = true; // Show the prince
    }

    // Ensure the camera is still looking at the prince
    camera.lookAt(prince.position);
  }
}

document.body.onscroll = moveCamera;
moveCamera();


// Game Loop function

const clock = new THREE.Clock();
function animate() {
  if(mixer)
      mixer.update(clock.getDelta());
    
  requestAnimationFrame( animate );

  prince.rotation.x += 0.005;
  rose.rotation.x += 0.01;
  baobab.rotation.x += 0.01;
  box.rotation.x += 0.01;
  sheep.rotation.x += 0.01;

  controls.update();
  
  renderer.render( scene, camera );
}
animate()

window.addEventListener("resize", function () {
  camera.aspect = window.innerWidth / window.innerHeight;
  camera.updateProjectionMatrix();
  renderer.setSize(window.innerWidth, window.innerHeight);
});